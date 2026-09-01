import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { api } from './api'
import type { Home, PlayerState, QueueItem, Song } from '../types'

type Ctx={home:Home|null;player:PlayerState|null;queue:QueueItem[];connected:boolean;error:string;refresh:()=>Promise<void>;play:(id:number)=>Promise<void>;stop:()=>Promise<void>;toggle:()=>Promise<void>;next:()=>Promise<void>;previous:()=>Promise<void>;seek:(ms:number)=>Promise<void>;volume:(v:number)=>Promise<void>;shuffle:()=>Promise<void>;repeat:()=>Promise<void>;addQueue:(id:number)=>Promise<void>;removeQueue:(id:number)=>Promise<void>;moveQueue:(id:number,direction:-1|1)=>Promise<void>;clearQueue:()=>Promise<void>;favorite:(song:Song)=>Promise<void>;playRadio:(id:number)=>Promise<void>}
const PlayerContext=createContext<Ctx|null>(null)
const message=(error:unknown,fallback:string)=>error instanceof Error&&error.name!=='AbortError'?error.message:fallback

export function PlayerProvider({children}:{children:ReactNode}){
  const [home,setHome]=useState<Home|null>(null),[player,setPlayer]=useState<PlayerState|null>(null),[queue,setQueue]=useState<QueueItem[]>([])
  const [connected,setConnected]=useState(false),[error,setError]=useState('')
  const playerRef=useRef<PlayerState|null>(null),commandID=useRef(0),refreshID=useRef(0),queueRequest=useRef<Promise<void>|null>(null)
  const updatePlayer=useCallback((value:PlayerState)=>{playerRef.current=value;setPlayer(value)},[])
  const refreshQueue=useCallback(()=>{
    if(queueRequest.current)return queueRequest.current
    const request=api.queue().then(setQueue).catch(e=>setError(message(e,'Could not update the queue'))).finally(()=>{if(queueRequest.current===request)queueRequest.current=null})
    queueRequest.current=request;return request
  },[])
  const syncPlayer=useCallback(async(signal?:AbortSignal)=>{try{const value=await api.player(signal);if(!signal?.aborted)updatePlayer(value)}catch(e){if(!signal?.aborted)setError(message(e,'Player unavailable'))}},[updatePlayer])
  const refresh=useCallback(async()=>{const id=++refreshID.current;try{const [h,q]=await Promise.all([api.home(),api.queue()]);if(id!==refreshID.current)return;setHome(h);updatePlayer(h.player);setQueue(q);setConnected(true);setError('')}catch(e){if(id===refreshID.current)setError(message(e,'Player unavailable'))}},[updatePlayer])
  useEffect(()=>{void refresh()},[refresh])
  useEffect(()=>{
    let ws:WebSocket|undefined,retry=0,timer:number|undefined,closed=false
    const connect=()=>{if(closed)return;const proto=location.protocol==='https:'?'wss':'ws';ws=new WebSocket(`${proto}://${location.host}/ws`);ws.onopen=()=>{retry=0;setConnected(true);setError('');void syncPlayer();void refreshQueue()};ws.onmessage=ev=>{try{const m=JSON.parse(ev.data);if(m.type==='player.state')updatePlayer(m.data);else if(m.type==='queue.changed')void refreshQueue();else if(m.type==='library.changed'||m.type==='playlist.changed')void api.home().then(setHome).catch(()=>{})}catch{/* Polling repairs malformed broadcasts. */}};ws.onerror=()=>ws?.close();ws.onclose=()=>{if(closed)return;setConnected(false);const delay=Math.min(750*1.8**retry++,12000);timer=window.setTimeout(connect,delay*(.8+Math.random()*.4))}}
    connect();return()=>{closed=true;if(timer)clearTimeout(timer);ws?.close()}
  },[refreshQueue,syncPlayer,updatePlayer])
  useEffect(()=>{const controller=new AbortController();const onVisible=()=>{if(document.visibilityState==='visible'){void syncPlayer(controller.signal);void refreshQueue()}};const onOnline=()=>void syncPlayer(controller.signal);document.addEventListener('visibilitychange',onVisible);window.addEventListener('online',onOnline);const id=window.setInterval(()=>{if(document.visibilityState==='visible')void syncPlayer(controller.signal)},15000);return()=>{controller.abort();document.removeEventListener('visibilitychange',onVisible);window.removeEventListener('online',onOnline);clearInterval(id)}},[refreshQueue,syncPlayer])
  const act=useCallback(async(fn:()=>Promise<PlayerState>)=>{const id=++commandID.current;try{const value=await fn();if(id===commandID.current){updatePlayer(value);setError('')}}catch(e){if(id===commandID.current)setError(message(e,'Command failed'))}},[updatePlayer])
  const play=useCallback((id:number)=>act(()=>api.play(id)),[act]),stop=useCallback(()=>act(()=>api.stop()),[act]),toggle=useCallback(()=>act(()=>api.pause()),[act]),next=useCallback(()=>act(()=>api.next()),[act]),previous=useCallback(()=>act(()=>api.previous()),[act]),seek=useCallback((ms:number)=>act(()=>api.seek(ms)),[act]),volume=useCallback((value:number)=>act(()=>api.volume(value)),[act])
  const shuffle=useCallback(()=>act(()=>api.shuffle(!playerRef.current?.shuffle)),[act])
  const repeat=useCallback(()=>{const current=playerRef.current?.repeat,next=current==='off'?'queue':current==='queue'?'track':'off';return act(()=>api.repeat(next))},[act])
  const addQueue=useCallback(async(id:number)=>{try{setQueue(await api.addQueue(id));setError('')}catch(e){setError(message(e,'Queue update failed'))}},[]),removeQueue=useCallback(async(id:number)=>{try{setQueue(await api.removeQueue(id));setError('')}catch(e){setError(message(e,'Queue update failed'))}},[]),moveQueue=useCallback(async(id:number,direction:-1|1)=>{try{const current=queue.findIndex(item=>item.id===id),target=current+direction;if(current<0||target<0||target>=queue.length)return;const ids=queue.map(item=>item.id);[ids[current],ids[target]]=[ids[target],ids[current]];setQueue(await api.reorderQueue(ids));setError('')}catch(e){setError(message(e,'Queue update failed'))}},[queue]),clearQueue=useCallback(async()=>{try{setQueue(await api.clearQueue());setError('')}catch(e){setError(message(e,'Queue update failed'))}},[])
  const favorite=useCallback(async(song:Song)=>{try{await api.favorite(song.id,!song.favorite);await refresh()}catch(e){setError(message(e,'Favorite failed'))}},[refresh]),playRadio=useCallback((id:number)=>act(()=>api.playRadio(id)),[act])
  const value=useMemo<Ctx>(()=>({home,player,queue,connected,error,refresh,play,stop,toggle,next,previous,seek,volume,shuffle,repeat,addQueue,removeQueue,moveQueue,clearQueue,favorite,playRadio}),[home,player,queue,connected,error,refresh,play,stop,toggle,next,previous,seek,volume,shuffle,repeat,addQueue,removeQueue,moveQueue,clearQueue,favorite,playRadio])
  return <PlayerContext.Provider value={value}>{children}</PlayerContext.Provider>
}
export function usePlayer(){const value=useContext(PlayerContext);if(!value)throw new Error('PlayerProvider missing');return value}
