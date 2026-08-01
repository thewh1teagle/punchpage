import {SimplePool} from 'nostr-tools/pool';
import {finalizeEvent, generateSecretKey, getPublicKey} from 'nostr-tools/pure';

const SIGNAL_KIND = 24242;
const DEFAULT_RELAYS = ['wss://relay.damus.io', 'wss://nos.lol', 'wss://relay.primal.net'];
const status = document.querySelector('#status');
const site = document.querySelector('#site');
const fragment = new URLSearchParams(location.hash.slice(1));
const room = fragment.get('r');
const keyText = fragment.get('k');
const relays = (fragment.get('relays') || DEFAULT_RELAYS.join(',')).split(',').filter(url => url.startsWith('wss://'));
const peer = crypto.randomUUID();
const clientToken = crypto.randomUUID();
const pool = new SimplePool({enableReconnect:true});
const nostrSecret = generateSecretKey();
const nostrPubkey = getPublicKey(nostrSecret);
const seenEvents = new Set();
const pending = new Map();
let nextID = 1;
let dc;
let sharedKey;

const appBase = new URL('./', location.href);
const scopePath = appBase.pathname.endsWith('/') ? appBase.pathname : appBase.pathname + '/';
const setStatus = text => { status.textContent = text; };

if (!room || !keyText || relays.length === 0) {
  setStatus('This PunchPage link is incomplete. Ask the host for a fresh link.');
} else {
  start().catch(error => setStatus('Connection failed: ' + error.message));
}

async function start() {
  sharedKey = await crypto.subtle.importKey('raw', decodeURL(keyText), 'AES-GCM', false, ['encrypt','decrypt']);
  const swURL = new URL('sw.js', appBase);
  const registration = await navigator.serviceWorker.register(swURL, {scope:scopePath});
  registration.update().catch(() => {});
  await navigator.serviceWorker.ready;
  if (!navigator.serviceWorker.controller) { location.reload(); return; }
  const version = await serviceWorkerVersion();
  if (version !== '1') {
    await new Promise(resolve => {
      const timer = setTimeout(resolve, 2500);
      navigator.serviceWorker.addEventListener('controllerchange', () => { clearTimeout(timer); resolve(); }, {once:true});
    });
    location.reload();
    return;
  }

  subscribeSignals();
  const pc = new RTCPeerConnection({iceServers:[
    {urls:'stun:stun.cloudflare.com:3478'},
    {urls:'stun:stun.l.google.com:19302'}
  ]});
  dc = pc.createDataChannel('http');
  wireDataChannel(dc);
  pc.onicecandidate = event => {
    if (event.candidate) publishSignal({type:'candidate', candidate:event.candidate.toJSON()}).catch(() => {});
  };
  pc.onconnectionstatechange = () => {
    if (['failed','disconnected','closed'].includes(pc.connectionState)) setStatus('Direct connection ' + pc.connectionState);
  };
  setStatus('Finding the host through public relays…');
  await pc.setLocalDescription(await pc.createOffer());

  let answered = false;
  let offerTimer;
  const remoteCandidates = [];
  async function acceptSignal(signal) {
    if (signal.type === 'answer' && !answered) {
      await pc.setRemoteDescription(signal.sdp);
      answered = true;
      clearInterval(offerTimer);
      for (const candidate of remoteCandidates.splice(0)) {
        try { await pc.addIceCandidate(candidate); } catch (_) {}
      }
    } else if (signal.type === 'candidate') {
      if (!pc.remoteDescription) remoteCandidates.push(signal.candidate);
      else try { await pc.addIceCandidate(signal.candidate); } catch (_) {}
    }
  }
  window.__punchpageAcceptSignal = acceptSignal;
  offerTimer = setInterval(() => {
    if (!answered) publishSignal({type:'offer', sdp:pc.localDescription}).catch(() => {});
  }, 2500);
  await publishSignal({type:'offer', sdp:pc.localDescription});
}

function subscribeSignals() {
  pool.subscribeMany(relays, {kinds:[SIGNAL_KIND], '#d':[room], since:Math.floor(Date.now()/1000)-30}, {
    onevent: async event => {
      if (event.pubkey === nostrPubkey || seenEvents.has(event.id)) return;
      seenEvents.add(event.id);
      try {
        const payload = JSON.parse(new TextDecoder().decode(await decrypt(event.content)));
        if (payload.role === 'host' && payload.peer === peer) await window.__punchpageAcceptSignal?.(payload.signal);
      } catch (_) {}
    }
  });
}

async function publishSignal(signal) {
  const content = await encrypt(new TextEncoder().encode(JSON.stringify({role:'browser', peer, signal})));
  const event = finalizeEvent({kind:SIGNAL_KIND, created_at:Math.floor(Date.now()/1000), tags:[['d',room]], content}, nostrSecret);
  const writes = pool.publish(relays, event);
  await Promise.any(writes);
}

function wireDataChannel(channel) {
  navigator.serviceWorker.addEventListener('message', event => {
    if (event.data?.type !== 'proxyFetch' || !event.ports[0]) return;
    const port = event.ports[0];
    const id = String(nextID++);
    pending.set(id, {port});
    port.onmessage = message => {
      if (message.data?.type === 'cancel') send({type:'request-cancel', id});
    };
    const request = event.data.request;
    const headers = {};
    for (const [name, value] of Object.entries(request.headers || {})) headers[name] = [value];
    if (request.prefix) headers['X-PunchPage-Prefix'] = [request.prefix];
    send({type:'request', id, url:request.url, method:request.method, headers});
    if (request.body) {
      const bytes = new Uint8Array(request.body);
      for (let offset=0; offset<bytes.length; offset+=32768) send({type:'request-body', id, data:encode(bytes.subarray(offset,offset+32768))});
    }
    send({type:'request-end', id});
  });

  addEventListener('message', event => {
    if (event.source !== site.contentWindow || !event.data?.type?.startsWith('pp-ws-')) return;
    const message = event.data;
    if (message.type === 'pp-ws-open') send({type:'ws-open', id:message.id, url:message.url, protocols:message.protocols || []});
    if (message.type === 'pp-ws-send') send({type:'ws-send', id:message.id, binary:message.binary, data:message.data});
    if (message.type === 'pp-ws-close') send({type:'ws-close', id:message.id, code:message.code, reason:message.reason});
  });

  channel.onmessage = event => {
    const message = JSON.parse(event.data);
    if (message.type.startsWith('response-')) {
      const request = pending.get(message.id);
      if (!request) return;
      if (message.type === 'response-start') {
        for (const cookie of message.cookies || []) applyCookie(cookie);
        request.port.postMessage({type:'start', status:message.status, headers:message.headers || {}});
      } else if (message.type === 'response-body') {
        const bytes = decode(message.data);
        request.port.postMessage({type:'body', data:bytes.buffer}, [bytes.buffer]);
      } else {
        request.port.postMessage({type:message.type === 'response-end' ? 'end' : 'error', error:message.error});
        pending.delete(message.id);
      }
    } else if (message.type.startsWith('ws-')) {
      site.contentWindow?.postMessage({...message, type:'pp-' + message.type}, location.origin);
    }
  };
  channel.onopen = async () => {
    setStatus('Direct channel connected; loading the site…');
    await registerTopClient();
    site.src = scopePath + '__punchpage__/' + encodeURIComponent(clientToken) + '/';
    site.onload = () => { status.style.display='none'; site.style.display='block'; };
  };
}

function send(message) { dc.send(JSON.stringify(message)); }
function encode(bytes) { let value=''; const array=new Uint8Array(bytes); for(let i=0;i<array.length;i+=0x8000)value+=String.fromCharCode(...array.subarray(i,i+0x8000)); return btoa(value); }
function decode(value) { return Uint8Array.from(atob(value||''), character=>character.charCodeAt(0)); }
function encodeURL(bytes) { return encode(bytes).replaceAll('+','-').replaceAll('/','_').replace(/=+$/,''); }
function decodeURL(value) { return decode(value.replaceAll('-','+').replaceAll('_','/') + '='.repeat((4-value.length%4)%4)); }

async function encrypt(plaintext) {
  const nonce=crypto.getRandomValues(new Uint8Array(12));
  const ciphertext=new Uint8Array(await crypto.subtle.encrypt({name:'AES-GCM',iv:nonce},sharedKey,plaintext));
  const sealed=new Uint8Array(nonce.length+ciphertext.length); sealed.set(nonce); sealed.set(ciphertext,nonce.length);
  return encodeURL(sealed);
}
async function decrypt(encoded) {
  const sealed=decodeURL(encoded); const nonce=sealed.slice(0,12);
  return new Uint8Array(await crypto.subtle.decrypt({name:'AES-GCM',iv:nonce},sharedKey,sealed.slice(12)));
}
function applyCookie(raw) {
  if (/;\s*httponly(?:;|$)/i.test(raw)) return;
  try { document.cookie=raw.replace(/;\s*domain=[^;]*/ig,'').replace(/;\s*samesite=none/ig,'; SameSite=Lax'); } catch (_) {}
}
function serviceWorkerVersion() {
  return new Promise(resolve => {
    const channel=new MessageChannel(); const timer=setTimeout(()=>resolve('old'),500);
    channel.port1.onmessage=event=>{clearTimeout(timer);resolve(event.data?.version||'old');};
    navigator.serviceWorker.controller.postMessage({type:'version'},[channel.port2]);
  });
}
function registerTopClient() {
  return new Promise(resolve => {
    const channel=new MessageChannel(); const timer=setTimeout(resolve,1000);
    channel.port1.onmessage=()=>{clearTimeout(timer);resolve();};
    navigator.serviceWorker.controller.postMessage({type:'registerTop',token:clientToken},[channel.port2]);
  });
}
