self.addEventListener('install', event => event.waitUntil(self.skipWaiting()));
self.addEventListener('activate', event => event.waitUntil(self.clients.claim()));

const scopeURL = new URL(self.registration.scope);
const scopePath = scopeURL.pathname.endsWith('/') ? scopeURL.pathname : scopeURL.pathname + '/';
const topByToken = new Map();
const ownerByClient = new Map();

self.addEventListener('message', event => {
  if (event.data?.type === 'version' && event.ports[0]) event.ports[0].postMessage({version:'1'});
  if (event.data?.type === 'registerTop' && event.data.token && event.source?.id) {
    topByToken.set(event.data.token, event.source.id);
    event.ports[0]?.postMessage({ok:true});
  }
});

self.addEventListener('fetch', event => {
  const url = new URL(event.request.url);
  if (url.origin !== self.location.origin || !url.pathname.startsWith(scopePath)) return;
  const relative = url.pathname.slice(scopePath.length);
  if (relative === 'sw.js' || relative === '__punchpage_runtime__.js' || relative.startsWith('assets/')) return;
  event.respondWith(route(event, url, relative));
});

async function route(event, url, relative) {
  if (relative === '' || relative === 'index.html') {
    const client = event.clientId ? await self.clients.get(event.clientId) : null;
    if (!client || client.frameType === 'top-level') return fetch(event.request);
  }
  let ownership = ownerByClient.get(event.clientId);
  const virtual = relative.match(/^__punchpage__\/([^/]+)\//);
  if (virtual) {
    const token = decodeURIComponent(virtual[1]);
    ownership = {ownerID:topByToken.get(token), token};
  }
  if (!ownership?.ownerID) {
    const client = event.clientId ? await self.clients.get(event.clientId) : null;
    if (client?.frameType === 'top-level') ownership = {ownerID:client.id, token:''};
  }
  if (ownership?.ownerID && event.resultingClientId) ownerByClient.set(event.resultingClientId, ownership);
  return proxy(event.request, url, relative, ownership);
}

async function proxy(request, requestURL, relative, ownership) {
  const top = ownership?.ownerID ? await self.clients.get(ownership.ownerID) : null;
  if (!top) return new Response('PunchPage browser session is unavailable', {status:503});

  const channel = new MessageChannel();
  const headers = {};
  for (const [name,value] of request.headers) headers[name]=value;
  let body = null;
  if (request.method !== 'GET' && request.method !== 'HEAD') body = await request.clone().arrayBuffer();
  let path = '/' + relative + requestURL.search;
  const virtual = relative.match(/^__punchpage__\/[^/]+\/(.*)$/);
  if (virtual) path = '/' + virtual[1] + requestURL.search;
  const prefix = ownership.token ? scopePath + '__punchpage__/' + encodeURIComponent(ownership.token) : scopePath;

  let streamController;
  const queued=[];
  let startResolve,startReject;
  const started=new Promise((resolve,reject)=>{startResolve=resolve;startReject=reject;});
  const stream=new ReadableStream({
    start(controller){streamController=controller;for(const chunk of queued.splice(0))controller.enqueue(chunk);},
    cancel(){channel.port1.postMessage({type:'cancel'});}
  });
  channel.port1.onmessage=event=>{
    const message=event.data;
    if(message.type==='start')startResolve(message);
    if(message.type==='body'){const bytes=new Uint8Array(message.data);if(streamController)streamController.enqueue(bytes);else queued.push(bytes);}
    if(message.type==='end')streamController?.close();
    if(message.type==='error'){const error=new Error(message.error||'direct request failed');startReject(error);streamController?.error(error);}
  };
  top.postMessage({type:'proxyFetch',request:{url:path,method:request.method,headers,body,prefix}},[channel.port2]);
  try{
    const metadata=await started;
    const responseHeaders=new Headers();
    for(const [name,values] of Object.entries(metadata.headers||{})){
      if(name.toLowerCase()==='set-cookie')continue;
      for(const value of values)responseHeaders.append(name,value);
    }
    const noBody=request.method==='HEAD'||[101,204,205,304].includes(metadata.status);
    return new Response(noBody?null:stream,{status:metadata.status,headers:responseHeaders});
  }catch(error){
    return new Response(String(error),{status:502,headers:{'content-type':'text/plain'}});
  }
}
