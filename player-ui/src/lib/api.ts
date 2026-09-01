import type { Album, Artist, Home, PlayerState, Playlist, QueueItem, RadioStation, SearchResults, Song } from '../types'

type RequestOptions=RequestInit&{timeout?:number}
async function req<T>(path:string, options:RequestOptions={}):Promise<T>{
  const {timeout=10000,signal,...init}=options
  const controller=new AbortController()
  const abort=()=>controller.abort(signal?.reason)
  if(signal?.aborted)abort();else signal?.addEventListener('abort',abort,{once:true})
  const timer=window.setTimeout(()=>controller.abort(new DOMException('Request timed out','TimeoutError')),timeout)
  const headers=new Headers(init.headers)
  if(init.body&&!headers.has('Content-Type'))headers.set('Content-Type','application/json')
  headers.set('Accept','application/json')
  try{
    const r=await fetch(path,{...init,headers,signal:controller.signal})
    if(!r.ok){
      const body=await r.json().catch(()=>null) as {error?:string}|null
      throw new Error(body?.error||`Request failed (${r.status})`)
    }
    if(r.status===204)return undefined as T
    return await r.json() as T
  }catch(error){
    if(error instanceof DOMException&&error.name==='AbortError'&&controller.signal.reason?.name==='TimeoutError')throw new Error('The player took too long to respond')
    throw error
  }finally{
    clearTimeout(timer);signal?.removeEventListener('abort',abort)
  }
}
export const api = {
  home:(signal?:AbortSignal)=>req<Home>('/api/v1/home',{signal}),
  player:(signal?:AbortSignal)=>req<PlayerState>('/api/v1/player',{signal}),
  songs:(signal?:AbortSignal)=>req<Song[]>('/api/v1/songs',{signal}),
  albums:(signal?:AbortSignal)=>req<Album[]>('/api/v1/albums',{signal}),
  artists:(signal?:AbortSignal)=>req<Artist[]>('/api/v1/artists',{signal}),
  playlists:(signal?:AbortSignal)=>req<Playlist[]>('/api/v1/playlists',{signal}),
  radio:(signal?:AbortSignal)=>req<RadioStation[]>('/api/v1/radio',{signal}),
  queue:(signal?:AbortSignal)=>req<QueueItem[]>('/api/v1/queue',{signal}),
  search:(q:string,signal?:AbortSignal)=>req<SearchResults>('/api/v1/search?q='+encodeURIComponent(q),{signal}),
  albumSongs:(id:number,signal?:AbortSignal)=>req<Song[]>(`/api/v1/albums/${id}/songs`,{signal}),
  artistSongs:(id:number,signal?:AbortSignal)=>req<Song[]>(`/api/v1/artists/${id}/songs`,{signal}),
  playlistSongs:(id:number,signal?:AbortSignal)=>req<Song[]>(`/api/v1/playlists/${id}/songs`,{signal}),
  playPlaylist:(id:number,shuffle:boolean)=>req<PlayerState>(`/api/v1/playlists/${id}/play`,{method:'POST',body:JSON.stringify({shuffle})}),
  play:(song_id:number)=>req<PlayerState>('/api/v1/player/play',{method:'POST',body:JSON.stringify({song_id})}),
  pause:()=>req<PlayerState>('/api/v1/player/pause',{method:'POST'}),
  stop:()=>req<PlayerState>('/api/v1/player/stop',{method:'POST'}),
  next:()=>req<PlayerState>('/api/v1/player/next',{method:'POST'}),
  previous:()=>req<PlayerState>('/api/v1/player/previous',{method:'POST'}),
  seek:(position_ms:number)=>req<PlayerState>('/api/v1/player/seek',{method:'POST',body:JSON.stringify({position_ms})}),
  volume:(volume:number)=>req<PlayerState>('/api/v1/player/volume',{method:'PUT',body:JSON.stringify({volume})}),
  shuffle:(shuffle:boolean)=>req<PlayerState>('/api/v1/player/shuffle',{method:'PUT',body:JSON.stringify({shuffle})}),
  repeat:(repeat:string)=>req<PlayerState>('/api/v1/player/repeat',{method:'PUT',body:JSON.stringify({repeat})}),
  mute:(muted:boolean)=>req<PlayerState>('/api/v1/player/mute',{method:'PUT',body:JSON.stringify({muted})}),
  playRadio:(id:number)=>req<PlayerState>(`/api/v1/radio/${id}/play`,{method:'POST'}),
  addQueue:(song_id:number)=>req<QueueItem[]>('/api/v1/queue',{method:'POST',body:JSON.stringify({song_id})}),
  clearQueue:()=>req<QueueItem[]>('/api/v1/queue',{method:'DELETE'}),
  removeQueue:(id:number)=>req<QueueItem[]>(`/api/v1/queue/${id}`,{method:'DELETE'}),
  reorderQueue:(ids:number[])=>req<QueueItem[]>('/api/v1/queue',{method:'PUT',body:JSON.stringify({ids})}),
  favorite:(id:number,value:boolean)=>req<Song>(`/api/v1/songs/${id}`,{method:'PATCH',body:JSON.stringify({favorite:value})})
}
