import { ArrowLeft, Check, SunMoon as CircleHalf, Moon, MoonStar, Sun } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { useTheme, type ThemeMode } from '../lib/theme'

const options:{id:ThemeMode;title:string;desc:string;icon:typeof Sun}[]=[
  {id:'auto',title:'Automatic',desc:'Light by day, dark in evening, dim Night mode after 10 PM.',icon:CircleHalf},
  {id:'light',title:'Light',desc:'Soft off-white surfaces and dark text for bright rooms.',icon:Sun},
  {id:'dark',title:'Dark',desc:'Low-glare charcoal surfaces with strong readable contrast.',icon:Moon},
  {id:'night',title:'Night',desc:'Very dark surfaces, dimmer whites and subdued artwork for dark rooms.',icon:MoonStar},
]

export function AppearancePage(){
  const {mode,setMode}=useTheme(),nav=useNavigate()
  return <div className="page settings-page">
    <header className="simple-header with-back"><button className="icon-button glass" aria-label="Back" onClick={()=>nav(-1)}><ArrowLeft/></button><h1>Appearance</h1></header>
    <p className="settings-intro">Choose how the controller looks. This only changes this browser; it never affects playback on the appliance.</p>
    <div className="appearance-list">{options.map(x=>{const Icon=x.icon;return <button key={x.id} className={mode===x.id?'selected':''} onClick={()=>setMode(x.id)}><span className="appearance-icon"><Icon/></span><span><strong>{x.title}</strong><small>{x.desc}</small></span>{mode===x.id&&<Check className="appearance-check"/>}</button>})}</div>
    <div className="night-preview"><span>Night mode preview</span><div><i/><i/><i/></div><strong>Easy on the room. Easy to read.</strong><small>Artwork and highlights are intentionally restrained instead of glowing at full brightness.</small></div>
  </div>
}
