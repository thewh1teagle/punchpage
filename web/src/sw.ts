/**
 * PunchPage service worker.
 *
 * Intercepts requests under the registration scope and hands them to the
 * controlling top-level page, which proxies them over the WebRTC data channel.
 * Built to `dist/sw.js` as a classic (non-module) worker script: it must stay
 * self-contained, so only type-only imports are allowed here.
 */
export {};

declare const self: ServiceWorkerGlobalScope;

/** Version handshake value; must match EXPECTED_SW_VERSION in main.ts. */
const VERSION = '1';

/**
 * Local copies of the wire types (mirrors of `src/protocol.ts`): this file is
 * typechecked with the WebWorker lib, so it cannot pull in DOM-typed modules.
 */
type WireHeaders = Record<string, string[]>;
type ProxyPortMessage =
  | {type: 'start'; status: number; headers: WireHeaders}
  | {type: 'body'; data: ArrayBuffer}
  | {type: 'end'}
  | {type: 'error'; error?: string};

/** Identifies the top-level client responsible for proxying a request. */
interface Ownership {
  ownerID: string | undefined;
  token: string;
}

self.addEventListener('install', event => event.waitUntil(self.skipWaiting()));
self.addEventListener('activate', event => event.waitUntil(self.clients.claim()));

const scopeURL = new URL(self.registration.scope);
const scopePath = scopeURL.pathname.endsWith('/') ? scopeURL.pathname : scopeURL.pathname + '/';
const topByToken = new Map<string, string>();
const ownerByClient = new Map<string, Ownership>();

self.addEventListener('message', event => {
  const data = event.data as {type?: string; token?: string} | undefined;
  const port = event.ports[0];
  if (data?.type === 'version' && port) port.postMessage({version: VERSION});
  if (data?.type === 'registerTop' && data.token && (event.source as Client | null)?.id) {
    topByToken.set(data.token, (event.source as Client).id);
    port?.postMessage({ok: true});
  }
});

self.addEventListener('fetch', event => {
  const url = new URL(event.request.url);
  if (url.origin !== self.location.origin || !url.pathname.startsWith(scopePath)) return;
  const relative = url.pathname.slice(scopePath.length);
  if (relative === 'sw.js' || relative === '__punchpage_runtime__.js' || relative.startsWith('assets/')) return;
  event.respondWith(route(event, url, relative));
});

/** Resolves which top-level client owns this request, then proxies it. */
async function route(event: FetchEvent, url: URL, relative: string): Promise<Response> {
  if (relative === '' || relative === 'index.html') {
    const client = event.clientId ? await self.clients.get(event.clientId) : null;
    if (!client || client.frameType === 'top-level') return fetch(event.request);
  }
  let ownership = ownerByClient.get(event.clientId);
  const virtual = relative.match(/^__punchpage__\/([^/]+)\//);
  if (virtual) {
    const token = decodeURIComponent(virtual[1] as string);
    ownership = {ownerID: topByToken.get(token), token};
  }
  if (!ownership?.ownerID) {
    const client = event.clientId ? await self.clients.get(event.clientId) : null;
    if (client?.frameType === 'top-level') ownership = {ownerID: client.id, token: ''};
  }
  if (ownership?.ownerID && event.resultingClientId) ownerByClient.set(event.resultingClientId, ownership);
  return proxy(event.request, url, relative, ownership);
}

/** Streams a request through the owning page and rebuilds the response. */
async function proxy(
  request: Request,
  requestURL: URL,
  relative: string,
  ownership: Ownership | undefined
): Promise<Response> {
  const top = ownership?.ownerID ? await self.clients.get(ownership.ownerID) : null;
  if (!top) return new Response('PunchPage browser session is unavailable', {status: 503});

  const channel = new MessageChannel();
  const headers: Record<string, string> = {};
  for (const [name, value] of request.headers) headers[name] = value;
  let body: ArrayBuffer | null = null;
  if (request.method !== 'GET' && request.method !== 'HEAD') body = await request.clone().arrayBuffer();
  let path = '/' + relative + requestURL.search;
  const virtual = relative.match(/^__punchpage__\/[^/]+\/(.*)$/);
  if (virtual) path = '/' + virtual[1] + requestURL.search;
  const prefix = ownership?.token
    ? scopePath + '__punchpage__/' + encodeURIComponent(ownership.token)
    : scopePath;

  let streamController: ReadableStreamDefaultController<Uint8Array> | undefined;
  const queued: Uint8Array[] = [];
  let startResolve!: (metadata: {status: number; headers?: WireHeaders}) => void;
  let startReject!: (error: Error) => void;
  const started = new Promise<{status: number; headers?: WireHeaders}>((resolve, reject) => {
    startResolve = resolve;
    startReject = reject;
  });
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      streamController = controller;
      for (const chunk of queued.splice(0)) controller.enqueue(chunk);
    },
    cancel() {
      channel.port1.postMessage({type: 'cancel'});
    }
  });
  channel.port1.onmessage = event => {
    const message = event.data as ProxyPortMessage;
    if (message.type === 'start') startResolve(message);
    if (message.type === 'body') {
      const bytes = new Uint8Array(message.data);
      if (streamController) streamController.enqueue(bytes);
      else queued.push(bytes);
    }
    if (message.type === 'end') streamController?.close();
    if (message.type === 'error') {
      const error = new Error(message.error || 'direct request failed');
      startReject(error);
      streamController?.error(error);
    }
  };
  top.postMessage({type: 'proxyFetch', request: {url: path, method: request.method, headers, body, prefix}}, [channel.port2]);
  try {
    const metadata = await started;
    const responseHeaders = new Headers();
    for (const [name, values] of Object.entries(metadata.headers || {})) {
      if (name.toLowerCase() === 'set-cookie') continue;
      for (const value of values) responseHeaders.append(name, value);
    }
    const noBody = request.method === 'HEAD' || [101, 204, 205, 304].includes(metadata.status);
    return new Response(noBody ? null : stream, {status: metadata.status, headers: responseHeaders});
  } catch (error) {
    return new Response(String(error), {status: 502, headers: {'content-type': 'text/plain'}});
  }
}
