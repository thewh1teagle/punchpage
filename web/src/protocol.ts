/** Shared wire-protocol types for the PunchPage browser client. */

/** WebRTC signaling payloads exchanged (encrypted) over Nostr relays. */
export type Signal =
  | { type: 'offer'; sdp: RTCSessionDescriptionInit }
  | { type: 'answer'; sdp: RTCSessionDescriptionInit }
  | { type: 'candidate'; candidate: RTCIceCandidateInit };

/** Envelope wrapped around every signal before encryption. */
export interface SignalEnvelope {
  role: 'browser' | 'host';
  peer: string;
  signal: Signal;
}

/** Multi-value HTTP headers as represented on the wire. */
export type WireHeaders = Record<string, string[]>;

/** Messages the browser client sends to the host over the data channel. */
export type ClientMessage =
  | { type: 'request'; id: string; url: string; method: string; headers: WireHeaders }
  | { type: 'request-body'; id: string; data: string }
  | { type: 'request-end'; id: string }
  | { type: 'request-cancel'; id: string }
  | { type: 'ws-open'; id: string; url: string; protocols: string[] }
  | { type: 'ws-send'; id: string; binary: boolean; data: string }
  | { type: 'ws-close'; id: string; code?: number; reason?: string };

/** Messages the host sends back over the data channel. */
export type HostMessage =
  | { type: 'response-start'; id: string; status: number; headers?: WireHeaders; cookies?: string[] }
  | { type: 'response-body'; id: string; data: string }
  | { type: 'response-end'; id: string }
  | { type: 'response-error'; id: string; error?: string }
  | HostSocketMessage;

/** WebSocket bridge messages relayed from the host to the sandboxed iframe. */
export type HostSocketMessage =
  | { type: 'ws-opened'; id: string; protocol?: string }
  | { type: 'ws-message'; id: string; binary: boolean; data: string }
  | { type: 'ws-error'; id: string; error?: string }
  | { type: 'ws-close'; id: string; code?: number; reason?: string };

/** postMessage payloads sent by the runtime shim inside the iframe. */
export type FrameSocketMessage =
  | { type: 'pp-ws-open'; id: string; url: string; protocols?: string[] }
  | { type: 'pp-ws-send'; id: string; binary: boolean; data: string }
  | { type: 'pp-ws-close'; id: string; code?: number; reason?: string };

/** Request description forwarded by the service worker for proxying. */
export interface ProxyFetchRequest {
  url: string;
  method: string;
  headers?: Record<string, string>;
  body?: ArrayBuffer | null;
  prefix?: string;
}

/** Message from the service worker asking the page to proxy a fetch. */
export interface ProxyFetchMessage {
  type: 'proxyFetch';
  request: ProxyFetchRequest;
}

/** Messages posted back to the service worker on the per-request port. */
export type ProxyPortMessage =
  | { type: 'start'; status: number; headers: WireHeaders }
  | { type: 'body'; data: ArrayBuffer }
  | { type: 'end' }
  | { type: 'error'; error?: string };

/** Message the service worker sends on the port to cancel a request. */
export interface ProxyCancelMessage {
  type: 'cancel';
}

/** State tracked for each in-flight proxied request. */
export interface PendingRequest {
  port: MessagePort;
}
