import { lazy, Suspense } from 'react'
import { BrowserRouter, Route, Routes } from 'react-router-dom'
import { Shell } from './components/Shell'
import { PlayerProvider } from './lib/player'
import { ThemeProvider } from './lib/theme'
const AlbumPage=lazy(()=>import('./pages/AlbumPage').then(x=>({default:x.AlbumPage})))
const AppearancePage=lazy(()=>import('./pages/AppearancePage').then(x=>({default:x.AppearancePage})))
const ArtistPage=lazy(()=>import('./pages/ArtistPage').then(x=>({default:x.ArtistPage})))
const HomePage=lazy(()=>import('./pages/HomePage').then(x=>({default:x.HomePage})))
const LibraryPage=lazy(()=>import('./pages/LibraryPage').then(x=>({default:x.LibraryPage})))
const NowPlayingPage=lazy(()=>import('./pages/NowPlayingPage').then(x=>({default:x.NowPlayingPage})))
const PlaylistPage=lazy(()=>import('./pages/PlaylistPage').then(x=>({default:x.PlaylistPage})))
const QueuePage=lazy(()=>import('./pages/QueuePage').then(x=>({default:x.QueuePage})))
const SearchPage=lazy(()=>import('./pages/SearchPage').then(x=>({default:x.SearchPage})))
export default function App(){return <ThemeProvider><PlayerProvider><BrowserRouter><Suspense fallback={<div className="page loading">Loading…</div>}><Routes><Route element={<Shell/>}><Route path="/" element={<HomePage/>}/><Route path="/search" element={<SearchPage/>}/><Route path="/library" element={<LibraryPage/>}/><Route path="/queue" element={<QueuePage/>}/></Route><Route path="/now-playing" element={<NowPlayingPage/>}/><Route path="/album/:id" element={<AlbumPage/>}/><Route path="/artist/:id" element={<ArtistPage/>}/><Route path="/playlist/:id" element={<PlaylistPage/>}/><Route path="/appearance" element={<AppearancePage/>}/></Routes></Suspense></BrowserRouter></PlayerProvider></ThemeProvider>}
