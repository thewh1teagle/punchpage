import {SecretBox} from './codec';
import {Signaling} from './signaling';
import {Tunnel} from './tunnel';
import type {Signal} from './protocol';
import {setStatus, showError, showSite, site} from './ui';

const DEFAULT_RELAYS = ['wss://relay.damus.io', 'wss://nos.lol', 'wss://relay.primal.net'];
const OFFER_RESEND_INTERVAL_MS = 2500;
const EXPECTED_SW_VERSION = '1';

const hash = location.hash.replace(/^#/, '').trim();
const fragment = new URLSearchParams(hash);
const room = fragment.get('r');
const keyText = fragment.get('k');
const relays = (fragment.get('relays') || DEFAULT_RELAYS.join(','))
  .split(',')
  .filter(url => url.startsWith('wss://'));
const peer = crypto.randomUUID();
const clientToken = crypto.randomUUID();

const appBase = new URL('./', location.href);
const scopePath = appBase.pathname.endsWith('/') ? appBase.pathname : appBase.pathname + '/';

if (hash === '') {
  // Plain root URL with no share fragment: this is the marketing/landing page.
  void import('./landing').then(({showLanding}) => showLanding());
} else if (!room || !keyText || relays.length === 0) {
  showError('This PunchPage link is incomplete. Ask the host for a fresh link.');
} else {
  start(room, keyText).catch((error: Error) => showError('Connection failed: ' + error.message));
}

async function start(room: string, keyText: string): Promise<void> {
  const box = await SecretBox.import(keyText);
  if (!(await ensureServiceWorker())) return;

  const signaling = new Signaling({relays, room, peer, box});
  const pc = new RTCPeerConnection({iceServers: [
    {urls: 'stun:stun.cloudflare.com:3478'},
    {urls: 'stun:stun.l.google.com:19302'}
  ]});
  new Tunnel(pc.createDataChannel('http'), site, onTunnelOpen);
  pc.onicecandidate = event => {
    if (event.candidate) signaling.publish({type: 'candidate', candidate: event.candidate.toJSON()}).catch(() => {});
  };
  pc.onconnectionstatechange = () => {
    if (['failed', 'disconnected', 'closed'].includes(pc.connectionState)) {
      showError('Direct connection ' + pc.connectionState);
    }
  };
  setStatus('Finding the host through public relays…');
  await pc.setLocalDescription(await pc.createOffer());
  const offer = pc.localDescription;
  if (!offer) throw new Error('missing local description');

  let answered = false;
  let offerTimer: ReturnType<typeof setInterval> | undefined;
  const remoteCandidates: RTCIceCandidateInit[] = [];
  async function acceptSignal(signal: Signal): Promise<void> {
    if (signal.type === 'answer' && !answered) {
      await pc.setRemoteDescription(signal.sdp);
      answered = true;
      clearInterval(offerTimer);
      for (const candidate of remoteCandidates.splice(0)) {
        try { await pc.addIceCandidate(candidate); } catch { /* stale candidate */ }
      }
    } else if (signal.type === 'candidate') {
      if (!pc.remoteDescription) remoteCandidates.push(signal.candidate);
      else try { await pc.addIceCandidate(signal.candidate); } catch { /* stale candidate */ }
    }
  }
  signaling.subscribe(acceptSignal);
  offerTimer = setInterval(() => {
    if (!answered) signaling.publish({type: 'offer', sdp: offer}).catch(() => {});
  }, OFFER_RESEND_INTERVAL_MS);
  await signaling.publish({type: 'offer', sdp: offer});
}

/** Registers the service worker; returns false when a reload is in flight. */
async function ensureServiceWorker(): Promise<boolean> {
  const swURL = new URL('sw.js', appBase);
  const registration = await navigator.serviceWorker.register(swURL, {scope: scopePath});
  registration.update().catch(() => {});
  await navigator.serviceWorker.ready;
  if (!navigator.serviceWorker.controller) {
    location.reload();
    return false;
  }
  if (await serviceWorkerVersion() !== EXPECTED_SW_VERSION) {
    await new Promise<void>(resolve => {
      const timer = setTimeout(resolve, 2500);
      navigator.serviceWorker.addEventListener('controllerchange', () => {
        clearTimeout(timer);
        resolve();
      }, {once: true});
    });
    location.reload();
    return false;
  }
  return true;
}

async function onTunnelOpen(): Promise<void> {
  setStatus('Direct channel connected; loading the site…');
  await registerTopClient();
  site.src = scopePath + '__punchpage__/' + encodeURIComponent(clientToken) + '/';
  site.onload = () => showSite();
}

function serviceWorkerVersion(): Promise<string> {
  return new Promise(resolve => {
    const channel = new MessageChannel();
    const timer = setTimeout(() => resolve('old'), 500);
    channel.port1.onmessage = event => {
      clearTimeout(timer);
      resolve((event.data as {version?: string} | undefined)?.version || 'old');
    };
    navigator.serviceWorker.controller?.postMessage({type: 'version'}, [channel.port2]);
  });
}

function registerTopClient(): Promise<void> {
  return new Promise(resolve => {
    const channel = new MessageChannel();
    const timer = setTimeout(resolve, 1000);
    channel.port1.onmessage = () => {
      clearTimeout(timer);
      resolve();
    };
    navigator.serviceWorker.controller?.postMessage({type: 'registerTop', token: clientToken}, [channel.port2]);
  });
}
