import { Outlet } from 'react-router-dom'
import { BottomNav } from './BottomNav'
import { MiniPlayer } from './MiniPlayer'
import { usePlayer } from '../lib/player'
export function Shell(){const {connected,error}=usePlayer();return <div className="app-shell"><main className="page-wrap"><Outlet/></main>{(!connected||error)&&<div className={`connection-banner ${connected?'error':''}`} role="status"><span/> {error||(navigator.onLine?'Reconnecting to Music Room…':'You are offline')}</div>}<div className="player-dock"><MiniPlayer/><BottomNav/></div></div>}
