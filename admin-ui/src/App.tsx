import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { Layout } from "./components/Layout";
import { Dashboard } from "./pages/Dashboard";
import { Folders } from "./pages/Folders";
import { Radio, Settings } from "./pages/ManagedLists";
import { Playlists } from "./pages/Playlists";
import { Artwork } from "./pages/Templates";
import { Torrents } from "./pages/Torrents";
import {Login} from './components/Login'
import {api,AdminSession} from './lib/api'
import {useEffect,useState} from 'react'
export default function App() {
  const [session,setSession]=useState<AdminSession|null|undefined>(undefined)
  useEffect(()=>{api.session().then(setSession).catch(()=>setSession(null));const reset=()=>setSession(null);window.addEventListener('ekkoplayer:auth-required',reset);return()=>window.removeEventListener('ekkoplayer:auth-required',reset)},[])
  if(session===undefined)return <div className="login-page"><div className="login-card"><p>Checking session…</p></div></div>
  if(session===null)return <Login onLogin={setSession}/>
  return (
    <BrowserRouter basename="/admin">
      <Routes>
        <Route element={<Layout />}>
          <Route path="/" element={<Dashboard />} />
          <Route path="/folders" element={<Folders />} />
          <Route path="/playlists" element={<Playlists />} />
          <Route path="/radio" element={<Radio />} />
          <Route path="/imports" element={<Navigate to="/folders" replace />} />
          <Route path="/torrents" element={<Torrents />} />
          <Route path="/artwork" element={<Artwork />} />
          <Route path="/settings" element={<Settings />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
