/**
 * Runtime shim injected into tunneled pages (see internal/tunnel/rewrite.go).
 *
 * Replaces `WebSocket` with a bridge that relays frames to the parent page and
 * rewrites same-origin `fetch` / `XMLHttpRequest` URLs onto the tunnel prefix.
 * Built to `dist/__punchpage_runtime__.js`; it runs as a classic script inside
 * the sandboxed iframe, so it must stay self-contained (type-only imports only).
 */
import type {FrameSocketMessage, HostSocketMessage} from './protocol';

export {};

declare global {
  interface Window {
    __PUNCHPAGE_PREFIX__?: string;
  }
}

/** Host socket messages as re-posted by the parent page, `pp-` prefixed. */
type PrefixedSocketMessage<T> = T extends {type: infer K extends string}
  ? Omit<T, 'type'> & {type: `pp-${K}`}
  : never;
type FrameInboundMessage = PrefixedSocketMessage<HostSocketMessage>;

(() => {
  const prefix = window.__PUNCHPAGE_PREFIX__ || '';
  let nextSocketID = 1;
  const sockets = new Map<string, P2PWebSocket>();
  const toBase64 = (bytes: ArrayBuffer | Uint8Array): string => {
    let result = '';
    const array = bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes);
    for (let i = 0; i < array.length; i += 0x8000) result += String.fromCharCode(...array.subarray(i, i + 0x8000));
    return btoa(result);
  };
  const fromBase64 = (text: string | undefined): Uint8Array =>
    Uint8Array.from(atob(text || ''), character => character.charCodeAt(0));

  class P2PWebSocket extends EventTarget {
    static CONNECTING = 0;
    static OPEN = 1;
    static CLOSING = 2;
    static CLOSED = 3;
    url: string;
    readyState: number;
    bufferedAmount: number;
    extensions: string;
    protocol: string;
    binaryType: 'blob' | 'arraybuffer';
    _id: string;
    [handler: string]: unknown;

    constructor(url: string | URL, protocols: string | string[] = []) {
      super();
      this.url = new URL(String(url), location.href).href;
      this.readyState = 0;
      this.bufferedAmount = 0;
      this.extensions = '';
      this.protocol = '';
      this.binaryType = 'blob';
      this._id = 'ws-' + nextSocketID++;
      sockets.set(this._id, this);
      const list = typeof protocols === 'string' ? [protocols] : Array.from(protocols);
      post({type: 'pp-ws-open', id: this._id, url: this.url, protocols: list});
    }

    send(data: string | ArrayBufferLike | ArrayBufferView | Blob): void {
      if (this.readyState !== 1) throw new DOMException('WebSocket is not open', 'InvalidStateError');
      if (typeof data === 'string') {
        post({type: 'pp-ws-send', id: this._id, binary: false, data: toBase64(new TextEncoder().encode(data))});
      } else if (data instanceof Blob) {
        data.arrayBuffer().then(buffer => post({type: 'pp-ws-send', id: this._id, binary: true, data: toBase64(buffer)}));
      } else {
        const buffer = ArrayBuffer.isView(data)
          ? (data.buffer.slice(data.byteOffset, data.byteOffset + data.byteLength) as ArrayBuffer)
          : (data as ArrayBuffer);
        post({type: 'pp-ws-send', id: this._id, binary: true, data: toBase64(buffer)});
      }
    }

    close(code = 1000, reason = ''): void {
      if (this.readyState >= 2) return;
      this.readyState = 2;
      post({type: 'pp-ws-close', id: this._id, code, reason});
    }

    _emit(type: string, event: Event): void {
      this.dispatchEvent(event);
      const handler = this['on' + type];
      if (typeof handler === 'function') handler.call(this, event);
    }
  }
  for (const name of ['CONNECTING', 'OPEN', 'CLOSING', 'CLOSED'] as const) {
    Object.defineProperty(P2PWebSocket.prototype, name, {value: P2PWebSocket[name]});
  }

  /** Posts a socket message to the controlling page. */
  function post(message: FrameSocketMessage): void {
    parent.postMessage(message, location.origin);
  }

  addEventListener('message', event => {
    const data = event.data as {type?: string} | undefined;
    if (event.source !== parent || !data?.type?.startsWith('pp-ws-')) return;
    const message = event.data as FrameInboundMessage;
    const socket = sockets.get(message.id);
    if (!socket) return;
    if (message.type === 'pp-ws-opened') {
      socket.protocol = message.protocol || '';
      socket.readyState = 1;
      socket._emit('open', new Event('open'));
    } else if (message.type === 'pp-ws-message') {
      const bytes = fromBase64(message.data);
      let data: string | ArrayBuffer | Blob = new TextDecoder().decode(bytes);
      if (message.binary) data = socket.binaryType === 'arraybuffer' ? (bytes.buffer as ArrayBuffer) : new Blob([bytes as BlobPart]);
      socket._emit('message', new MessageEvent('message', {data}));
    } else if (message.type === 'pp-ws-error') {
      socket._emit('error', new Event('error'));
      socket.readyState = 3;
      socket._emit('close', new CloseEvent('close', {code: 1006, reason: message.error || '', wasClean: false}));
      sockets.delete(message.id);
    } else if (message.type === 'pp-ws-close') {
      socket.readyState = 3;
      socket._emit('close', new CloseEvent('close', {code: message.code || 1000, reason: message.reason || '', wasClean: (message.code || 1000) === 1000}));
      sockets.delete(message.id);
    }
  });
  window.WebSocket = P2PWebSocket as unknown as typeof WebSocket;

  /** Rewrites localhost / same-origin absolute URLs onto the tunnel prefix. */
  const localURL = <T extends string | URL>(raw: T): T | string => {
    const url = new URL(String(raw), location.href);
    if (['localhost', '127.0.0.1'].includes(url.hostname)) return prefix + url.pathname + url.search + url.hash;
    if (url.origin === location.origin && url.pathname.startsWith('/') && !url.pathname.startsWith(prefix + '/')) {
      return prefix + url.pathname + url.search + url.hash;
    }
    return raw;
  };
  const nativeFetch = window.fetch.bind(window);
  window.fetch = (input: RequestInfo | URL, init?: RequestInit) => {
    if (input instanceof Request) {
      const replacement = localURL(input.url);
      if (replacement !== input.url) input = new Request(replacement, input);
    } else {
      input = localURL(input);
    }
    return nativeFetch(input, init);
  };
  const nativeOpen = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function (this: XMLHttpRequest, method: string, url: string | URL, ...rest: unknown[]) {
    return (nativeOpen as (...args: unknown[]) => void).call(this, method, localURL(url), ...rest);
  } as typeof XMLHttpRequest.prototype.open;
})();
