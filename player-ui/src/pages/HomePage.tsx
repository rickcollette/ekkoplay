import { ChevronRight, MoonStar, Radio, Waves } from 'lucide-react'
import { Link, useNavigate } from 'react-router-dom'
import { Artwork } from '../components/Artwork'
import { TrackRow } from '../components/TrackRow'
import { usePlayer } from '../lib/player'
import { useTheme } from '../lib/theme'

export function HomePage(){const {home,player,connected,playRadio}=usePlayer(),{resolved}=useTheme(),nav=useNavigate();const current=player?.current_song;return <div className="page home-page">
  <header className="topbar"><div><div className="eyebrow"><span className={connected?'status-dot online':'status-dot'}/> MUSIC ROOM</div><h1>Good {greeting()}</h1></div><button className="icon-button glass" onClick={()=>nav('/appearance')} aria-label="Appearance"><MoonStar/></button></header>
  {current&&<button className="now-card" aria-label={`Open now playing: ${current.title}`} onClick={()=>nav('/now-playing')}><Artwork artwork={current.artwork} label={current.album} className="now-card-art"/><span className="now-card-copy"><span>NOW PLAYING</span><strong>{current.title}</strong><small>{current.artist} · {current.album}</small><span className="now-card-line"><i style={{width:`${Math.min(100,(player!.position_ms/(player!.duration_ms||1))*100)}%`}}/></span></span><ChevronRight/></button>}
  <Section title="Recently played" to="/library"><CardRail items={(home?.recently_played||[]).slice(0,6).map(s=>({id:s.id,title:s.album,sub:s.artist,art:s.artwork,onClick:()=>nav(`/album/${s.album_id}`)}))}/></Section>
  <Section title="Your playlists" to="/library?tab=playlists"><CardRail items={(home?.playlists||[]).map(p=>({id:p.id,title:p.name,sub:`${p.song_count} songs`,art:p.artwork,onClick:()=>nav(`/playlist/${p.id}`)}))}/></Section>
  <Section title="Recently added" to="/library?tab=songs"><div className="track-stack">{(home?.recently_added||[]).slice(0,4).map(s=><TrackRow key={s.id} song={s}/>)}</div></Section>
  <section className="home-radio"><div className="section-heading"><div><span className="section-kicker"><Radio/> LIVE</span><h2>Radio</h2></div><Link to="/library?tab=radio">See all <ChevronRight/></Link></div><div className="station-stack">{(home?.radio||[]).slice(0,3).map(r=><button key={r.id} className="station-card" aria-label={`Play ${r.name}`} onClick={()=>void playRadio(r.id)}><Artwork artwork={r.artwork} label={r.name} className="station-art"/><span><strong>{r.name}</strong><small>{r.genre}</small></span><Waves className="station-wave" aria-hidden="true"/></button>)}</div></section>
  <div className="mode-note">{resolved==='night'?'Night mode · artwork and whites are dimmed':resolved==='dark'?'Dark mode':'Light mode'}</div>
</div>}
function greeting(){const h=new Date().getHours();return h<12?'morning':h<18?'afternoon':'evening'}
function Section({title,to,children}:{title:string;to:string;children:React.ReactNode}){return <section><div className="section-heading"><h2>{title}</h2><Link to={to}>See all <ChevronRight/></Link></div>{children}</section>}
function CardRail({items}:{items:{id:number;title:string;sub:string;art?:string;onClick:()=>void}[]}){return <div className="card-rail">{items.map(x=><button className="media-card" key={x.id} onClick={x.onClick}><Artwork artwork={x.art} label={x.title} className="media-card-art"/><strong>{x.title}</strong><small>{x.sub}</small></button>)}</div>}
