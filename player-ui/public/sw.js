const CACHE='ekkoplayer-ui-v2'
const SHELL=['/','/manifest.webmanifest','/icon.svg']
self.addEventListener('install',event=>{event.waitUntil(caches.open(CACHE).then(cache=>cache.addAll(SHELL)).then(()=>self.skipWaiting()))})
self.addEventListener('activate',event=>{event.waitUntil(Promise.all([caches.keys().then(keys=>Promise.all(keys.filter(key=>key!==CACHE).map(key=>caches.delete(key)))),self.clients.claim()]))})
self.addEventListener('fetch',event=>{
  const request=event.request,url=new URL(request.url)
  if(request.method!=='GET'||url.origin!==self.location.origin||url.pathname.startsWith('/api/')||url.pathname==='/ws')return
  if(request.mode==='navigate'){
    event.respondWith(fetch(request).then(response=>{const copy=response.clone();void caches.open(CACHE).then(cache=>cache.put('/',copy));return response}).catch(()=>caches.match('/')))
    return
  }
  event.respondWith(caches.match(request).then(cached=>{const fresh=fetch(request).then(response=>{if(response.ok)void caches.open(CACHE).then(cache=>cache.put(request,response.clone()));return response}).catch(()=>cached);return cached||fresh}))
})
