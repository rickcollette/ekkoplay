import { Heart, MoreHorizontal, Play, Plus } from 'lucide-react'
import { useEffect, useState } from 'react'
import type { Song } from '../types'
import { duration } from '../lib/format'
import { Artwork } from './Artwork'
import { usePlayer } from '../lib/player'

export function TrackRow({song,index,compact=false}:{song:Song;index?:number;compact?:boolean}){
  const p=usePlayer(),[open,setOpen]=useState(false)
  useEffect(()=>{if(!open)return;const close=(e:KeyboardEvent)=>{if(e.key==='Escape')setOpen(false)};window.addEventListener('keydown',close);return()=>window.removeEventListener('keydown',close)},[open])
  return <>
    <div className={`track-row ${compact?'compact':''}`}>
      {index!==undefined?<div className="track-index">{index+1}</div>:<Artwork artwork={song.artwork} label={song.album} className="track-art"/>}
      <button className="track-main" onClick={()=>void p.play(song.id)}><strong>{song.title}</strong><span>{song.artist}{!compact&&` · ${song.album}`}</span></button>
      <button className="icon-button" aria-label="More actions" onClick={()=>setOpen(true)}><MoreHorizontal/></button>
    </div>
    {open&&<div className="sheet-backdrop" onClick={()=>setOpen(false)}><div className="action-sheet" role="dialog" aria-modal="true" aria-label={`Actions for ${song.title}`} onClick={e=>e.stopPropagation()}>
      <div className="sheet-grabber"/><div className="sheet-song"><Artwork artwork={song.artwork} label={song.album} className="track-art"/><div><strong>{song.title}</strong><span>{song.artist} · {duration(song.duration_ms)}</span></div></div>
      <button onClick={()=>{void p.play(song.id);setOpen(false)}}><Play/>Play now</button>
      <button onClick={()=>{void p.addQueue(song.id);setOpen(false)}}><Plus/>Add to queue</button>
      <button onClick={()=>{void p.favorite(song);setOpen(false)}}><Heart fill={song.favorite?'currentColor':'none'}/>{song.favorite?'Remove favorite':'Favorite'}</button>
      <button className="sheet-cancel" onClick={()=>setOpen(false)}>Cancel</button>
    </div></div>}
  </>
}
