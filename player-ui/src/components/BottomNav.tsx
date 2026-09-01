import { Home, Library, ListMusic, Search } from 'lucide-react'
import { NavLink } from 'react-router-dom'
const items=[['/','Home',Home],['/search','Search',Search],['/library','Library',Library],['/queue','Queue',ListMusic]] as const
export function BottomNav(){return <nav className="bottom-nav">{items.map(([to,label,Icon])=><NavLink key={to} to={to} end={to==='/'}><Icon/><span>{label}</span></NavLink>)}</nav>}
