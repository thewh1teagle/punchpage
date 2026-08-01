(() => {
  const prefix = window.__PUNCHPAGE_PREFIX__ || '';
  let nextSocketID = 1;
  const sockets = new Map();
  const toBase64 = bytes => {
    let result=''; const array=new Uint8Array(bytes);
    for(let i=0;i<array.length;i+=0x8000)result+=String.fromCharCode(...array.subarray(i,i+0x8000));
    return btoa(result);
  };
  const fromBase64 = text => Uint8Array.from(atob(text||''),character=>character.charCodeAt(0));

  class P2PWebSocket extends EventTarget {
    static CONNECTING=0; static OPEN=1; static CLOSING=2; static CLOSED=3;
    constructor(url,protocols=[]){
      super(); this.url=new URL(url,location.href).href; this.readyState=0; this.bufferedAmount=0;
      this.extensions=''; this.protocol=''; this.binaryType='blob'; this._id='ws-'+nextSocketID++;
      sockets.set(this._id,this);
      const list=typeof protocols==='string'?[protocols]:Array.from(protocols);
      parent.postMessage({type:'pp-ws-open',id:this._id,url:this.url,protocols:list},location.origin);
    }
    send(data){
      if(this.readyState!==1)throw new DOMException('WebSocket is not open','InvalidStateError');
      if(typeof data==='string')parent.postMessage({type:'pp-ws-send',id:this._id,binary:false,data:toBase64(new TextEncoder().encode(data))},location.origin);
      else if(data instanceof Blob)data.arrayBuffer().then(buffer=>parent.postMessage({type:'pp-ws-send',id:this._id,binary:true,data:toBase64(buffer)},location.origin));
      else{const buffer=ArrayBuffer.isView(data)?data.buffer.slice(data.byteOffset,data.byteOffset+data.byteLength):data;parent.postMessage({type:'pp-ws-send',id:this._id,binary:true,data:toBase64(buffer)},location.origin);}
    }
    close(code=1000,reason=''){if(this.readyState>=2)return;this.readyState=2;parent.postMessage({type:'pp-ws-close',id:this._id,code,reason},location.origin);}
    _emit(type,event){this.dispatchEvent(event);const handler=this['on'+type];if(typeof handler==='function')handler.call(this,event);}
  }
  for(const name of ['CONNECTING','OPEN','CLOSING','CLOSED'])Object.defineProperty(P2PWebSocket.prototype,name,{value:P2PWebSocket[name]});
  addEventListener('message',event=>{
    if(event.source!==parent||!event.data?.type?.startsWith('pp-ws-'))return;
    const message=event.data,socket=sockets.get(message.id);if(!socket)return;
    if(message.type==='pp-ws-opened'){socket.protocol=message.protocol||'';socket.readyState=1;socket._emit('open',new Event('open'));}
    else if(message.type==='pp-ws-message'){const bytes=fromBase64(message.data);let data=new TextDecoder().decode(bytes);if(message.binary)data=socket.binaryType==='arraybuffer'?bytes.buffer:new Blob([bytes]);socket._emit('message',new MessageEvent('message',{data}));}
    else if(message.type==='pp-ws-error'){socket._emit('error',new Event('error'));socket.readyState=3;socket._emit('close',new CloseEvent('close',{code:1006,reason:message.error||'',wasClean:false}));sockets.delete(message.id);}
    else if(message.type==='pp-ws-close'){socket.readyState=3;socket._emit('close',new CloseEvent('close',{code:message.code||1000,reason:message.reason||'',wasClean:(message.code||1000)===1000}));sockets.delete(message.id);}
  });
  window.WebSocket=P2PWebSocket;

  const localURL = raw => {
    const url=new URL(String(raw),location.href);
    if(['localhost','127.0.0.1'].includes(url.hostname))return prefix+url.pathname+url.search+url.hash;
    if(url.origin===location.origin&&url.pathname.startsWith('/')&&!url.pathname.startsWith(prefix+'/'))return prefix+url.pathname+url.search+url.hash;
    return raw;
  };
  const nativeFetch=window.fetch.bind(window);
  window.fetch=(input,init)=>{
    if(input instanceof Request){const replacement=localURL(input.url);if(replacement!==input.url)input=new Request(replacement,input);}
    else input=localURL(input);
    return nativeFetch(input,init);
  };
  const nativeOpen=XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open=function(method,url,...rest){return nativeOpen.call(this,method,localURL(url),...rest);};
})();
