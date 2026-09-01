import { Search, X } from 'lucide-react'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { Artwork } from '../components/Artwork'
import { TrackRow } from '../components/TrackRow'
import { api } from '../lib/api'
import { usePlayer } from '../lib/player'
import type { SearchResults } from '../types'
const empty:SearchResults={songs:[],albums:[],artists:[],playlists:[],radio:[]}
export function SearchPage(){
  const [q,setQ]=useState(''),[results,setResults]=useState<SearchResults>(empty),[loading,setLoading]=useState(false),[failed,setFailed]=useState(false),nav=useNavigate(),player=usePlayer(),ref=useRef<HTMLInputElement>(null)
  useEffect(()=>ref.current?.focus(),[])
  useEffect(()=>{const query=q.trim();if(query.length<2){setResults(empty);setLoading(false);setFailed(false);return}const controller=new AbortController(),id=window.setTimeout(()=>{setLoading(true);setFailed(false);void api.search(query,controller.signal).then(setResults).catch(e=>{if(e?.name!=='AbortError'){setResults(empty);setFailed(true)}}).finally(()=>{if(!controller.signal.aborted)setLoading(false)})},220);return()=>{clearTimeout(id);controller.abort()}},[q])
  const has=Object.values(results).some(x=>x.length),query=q.trim()
  return <div className="page"><header className="simple-header"><h1>Search</h1></header><div className="search-box"><Search/><input ref={ref} type="search" value={q} onChange={e=>setQ(e.target.value)} placeholder="Songs, artists, albums, playlists…" aria-label="Search library"/>{q&&<button onClick={()=>setQ('')} aria-label="Clear search"><X/></button>}</div>
    {query.length<2?<Empty icon title="Find anything in your music room">One search covers the whole local library and radio list.</Empty>:loading?<Empty title="Searching…"/>:failed?<Empty title="Search unavailable">Check the player connection and try again.</Empty>:!has?<Empty title="No matches">Try a different title, artist, album, or station.</Empty>:<div className="search-results">
      {results.songs.length>0&&<Result title="Songs"><div className="track-stack">{results.songs.map(s=><TrackRow key={s.id} song={s}/>)}</div></Result>}
      {results.artists.length>0&&<Result title="Artists"><div className="entity-list">{results.artists.map(a=><button key={a.id} onClick={()=>nav(`/artist/${a.id}`)}><Artwork artwork={a.artwork} label={a.name} className="entity-art circle"/><span><strong>{a.name}</strong><small>{a.song_count} songs</small></span></button>)}</div></Result>}
      {results.albums.length>0&&<Result title="Albums"><div className="album-grid">{results.albums.map(a=><button className="album-tile" key={a.id} onClick={()=>nav(`/album/${a.id}`)}><Artwork artwork={a.artwork} label={a.title} className="album-art"/><strong>{a.title}</strong><small>{a.artist}</small></button>)}</div></Result>}
      {results.playlists.length>0&&<Result title="Playlists"><div className="entity-list">{results.playlists.map(p=><button key={p.id} onClick={()=>nav(`/playlist/${p.id}`)}><Artwork artwork={p.artwork} label={p.name} className="entity-art"/><span><strong>{p.name}</strong><small>{p.song_count} songs</small></span></button>)}</div></Result>}
      {results.radio.length>0&&<Result title="Radio"><div className="entity-list">{results.radio.map(r=><button key={r.id} onClick={()=>void player.playRadio(r.id)}><Artwork artwork={r.artwork} label={r.name} className="entity-art"/><span><strong>{r.name}</strong><small>{r.genre}</small></span></button>)}</div></Result>}
    </div>}
  </div>
}
function Empty({title,children,icon=false}:{title:string;children?:ReactNode;icon?:boolean}){return <div className="empty-state compact" aria-live="polite">{icon&&<Search/>}<strong>{title}</strong>{children&&<span>{children}</span>}</div>}
function Result({title,children}:{title:string;children:ReactNode}){return <section><div className="section-heading"><h2>{title}</h2></div>{children}</section>}
