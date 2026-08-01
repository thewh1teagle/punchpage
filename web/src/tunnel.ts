import {decode, encode} from './codec';
import type {
  ClientMessage,
  FrameSocketMessage,
  HostMessage,
  PendingRequest,
  ProxyCancelMessage,
  ProxyFetchMessage,
  ProxyPortMessage,
  WireHeaders
} from './protocol';

const BODY_CHUNK_SIZE = 32768;

/**
 * Bridges the WebRTC data channel to the service worker (proxied HTTP
 * requests) and to the sandboxed iframe (proxied WebSockets).
 */
export class Tunnel {
  private readonly pending = new Map<string, PendingRequest>();
  private nextID = 1;

  constructor(
    private readonly channel: RTCDataChannel,
    private readonly frame: HTMLIFrameElement,
    onOpen: () => void
  ) {
    navigator.serviceWorker.addEventListener('message', event => this.onServiceWorkerMessage(event));
    addEventListener('message', event => this.onFrameMessage(event));
    channel.onmessage = event => this.onChannelMessage(JSON.parse(event.data as string) as HostMessage);
    channel.onopen = onOpen;
  }

  private send(message: ClientMessage): void {
    this.channel.send(JSON.stringify(message));
  }

  /** Handles a proxied fetch request forwarded by the service worker. */
  private onServiceWorkerMessage(event: MessageEvent): void {
    const data = event.data as ProxyFetchMessage | undefined;
    const port = event.ports[0];
    if (data?.type !== 'proxyFetch' || !port) return;
    const id = String(this.nextID++);
    this.pending.set(id, {port});
    port.onmessage = message => {
      if ((message.data as ProxyCancelMessage | undefined)?.type === 'cancel') this.send({type: 'request-cancel', id});
    };
    const request = data.request;
    const headers: WireHeaders = {};
    for (const [name, value] of Object.entries(request.headers || {})) headers[name] = [value];
    if (request.prefix) headers['X-PunchPage-Prefix'] = [request.prefix];
    this.send({type: 'request', id, url: request.url, method: request.method, headers});
    if (request.body) {
      const bytes = new Uint8Array(request.body);
      for (let offset = 0; offset < bytes.length; offset += BODY_CHUNK_SIZE) {
        this.send({type: 'request-body', id, data: encode(bytes.subarray(offset, offset + BODY_CHUNK_SIZE))});
      }
    }
    this.send({type: 'request-end', id});
  }

  /** Forwards WebSocket bridge messages from the sandboxed iframe to the host. */
  private onFrameMessage(event: MessageEvent): void {
    if (event.source !== this.frame.contentWindow) return;
    const message = event.data as Partial<FrameSocketMessage> | undefined;
    if (typeof message?.type !== 'string' || !message.type.startsWith('pp-ws-')) return;
    const socket = message as FrameSocketMessage;
    if (socket.type === 'pp-ws-open') {
      this.send({type: 'ws-open', id: socket.id, url: socket.url, protocols: socket.protocols || []});
    } else if (socket.type === 'pp-ws-send') {
      this.send({type: 'ws-send', id: socket.id, binary: socket.binary, data: socket.data});
    } else if (socket.type === 'pp-ws-close') {
      this.send({type: 'ws-close', id: socket.id, code: socket.code, reason: socket.reason});
    }
  }

  /** Dispatches host messages to the waiting service worker port or the iframe. */
  private onChannelMessage(message: HostMessage): void {
    if (message.type.startsWith('response-')) {
      const response = message as Extract<HostMessage, {type: `response-${string}`}>;
      const request = this.pending.get(response.id);
      if (!request) return;
      if (response.type === 'response-start') {
        for (const cookie of response.cookies || []) applyCookie(cookie);
        this.post(request.port, {type: 'start', status: response.status, headers: response.headers || {}});
      } else if (response.type === 'response-body') {
        const bytes = decode(response.data);
        request.port.postMessage({type: 'body', data: bytes.buffer} satisfies ProxyPortMessage, [bytes.buffer]);
      } else {
        this.post(request.port, response.type === 'response-end'
          ? {type: 'end'}
          : {type: 'error', error: response.error});
        this.pending.delete(response.id);
      }
    } else if (message.type.startsWith('ws-')) {
      this.frame.contentWindow?.postMessage({...message, type: 'pp-' + message.type}, location.origin);
    }
  }

  private post(port: MessagePort, message: ProxyPortMessage): void {
    port.postMessage(message);
  }
}

/** Mirrors host cookies into the page, dropping attributes that cannot apply. */
function applyCookie(raw: string): void {
  if (/;\s*httponly(?:;|$)/i.test(raw)) return;
  try {
    document.cookie = raw.replace(/;\s*domain=[^;]*/gi, '').replace(/;\s*samesite=none/gi, '; SameSite=Lax');
  } catch {
    // Ignore cookies the browser refuses.
  }
}
