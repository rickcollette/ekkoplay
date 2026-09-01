import { Download, Trash2, Upload } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api, type TorrentJob } from "../lib/api";
import { useLiveEvents } from "../lib/useLiveEvents";
import { Page } from "./Dashboard";

const bytes=(n:number)=>n?`${(n/1024/1024).toFixed(n>1024**3?0:1)} MB`:"0 MB";
const rate=(n:number)=>n?`${bytes(n)}/s`:"0 MB/s";
const instant=(v?:string)=>v?new Date(v.includes("T")?v:v.replace(" ","T")+"Z").getTime():0;
const date=(v?:string)=>v?new Date(instant(v)).toLocaleString():"—";
const duration=(ms:number)=>{if(ms<=0)return "ended";const days=Math.floor(ms/86400000),hours=Math.floor(ms%86400000/3600000),minutes=Math.floor(ms%3600000/60000);return days?`${days}d ${hours}h ${minutes}m`:hours?`${hours}h ${minutes}m`:`${minutes}m`};
const label=(s:TorrentJob["status"])=>({adding:"Adding",downloading:"Downloading",importing:"Importing",seeding:"Seeding",expired:"Seed period ended",failed:"Failed",deleted:"Original deleted"})[s];

export function Torrents(){
 const [jobs,setJobs]=useState<TorrentJob[]>([]),[folder,setFolder]=useState(""),[file,setFile]=useState<File>(),[busy,setBusy]=useState(false),[upload,setUpload]=useState(0),[error,setError]=useState(""),[now,setNow]=useState(Date.now());
 const input=useRef<HTMLInputElement>(null);
 const load=useCallback(()=>api.torrents().then(setJobs).catch(e=>setError(String(e))),[]);
 useEffect(()=>{void load()},[load]);
 useEffect(()=>{const timer=window.setInterval(()=>setNow(Date.now()),1000);return()=>window.clearInterval(timer)},[]);
 useLiveEvents(()=>void load(),["torrent.changed","import.changed"],500);
 const active=useMemo(()=>jobs.filter(j=>j.status!=="deleted"),[jobs]);
 const summary=useMemo(()=>({down:active.reduce((n,j)=>n+j.download_rate,0),up:active.reduce((n,j)=>n+j.upload_rate,0),peers:active.reduce((n,j)=>n+j.peers,0),bytes:active.reduce((n,j)=>n+j.total_bytes,0),seeding:active.filter(j=>j.status==="seeding").length}),[active]);
 const submit=async()=>{if(!file||!folder.trim())return;setBusy(true);setError("");try{await api.addTorrent(file,folder.trim(),p=>setUpload(p.percent));setFile(undefined);setFolder("");setUpload(0);if(input.current)input.current.value="";await load()}catch(e){setError(String(e))}finally{setBusy(false)}};
 const remove=async(j:TorrentJob)=>{if(!confirm(`Delete the original torrent data for “${j.name}”? Imported music is retained.`))return;try{await api.deleteTorrent(j.id);await load()}catch(e){setError(String(e))}};
 return <Page title="Torrents" sub="Download releases, import supported audio into a named music folder, and retain the original torrent for 14 days.">
  <div className="torrent-metrics"><div><span>Tracked torrents</span><strong>{active.length}</strong><small>{summary.seeding} seeding</small></div><div><span>Download speed</span><strong>{rate(summary.down)}</strong><small>combined current rate</small></div><div><span>Upload speed</span><strong>{rate(summary.up)}</strong><small>combined current rate</small></div><div><span>Connected peers</span><strong>{summary.peers}</strong><small>{bytes(summary.bytes)} original data</small></div></div>
  <section className="panel torrent-add"><div className="panel-title"><div><span>NEW DOWNLOAD</span><h2>Add a .torrent file</h2></div><Download/></div><div className="torrent-form"><label>Torrent file<input ref={input} type="file" accept=".torrent,application/x-bittorrent" onChange={e=>setFile(e.target.files?.[0])}/></label><label>Music folder name<input placeholder="Example: Pink Floyd" value={folder} onChange={e=>setFolder(e.target.value)}/></label><button className="primary" disabled={busy||!file||!folder.trim()} onClick={()=>void submit()}><Upload/>{busy?`Uploading ${upload}%`:"Upload and download"}</button></div>{error&&<p className="form-error">{error}</p>}</section>
  <section className="panel torrent-list"><div className="panel-title"><div><span>LIVE ACTIVITY</span><h2>Downloads, imports and seeding</h2></div></div>{jobs.map(j=>{const left=instant(j.seed_until)-now,ratio=j.downloaded_bytes?j.uploaded_bytes/j.downloaded_bytes:0;return <article className={`torrent-row status-${j.status}`} key={j.id}><div className="torrent-heading"><div><strong>{j.name}</strong><span>Music / {j.target_folder} · {label(j.status)}</span></div><b>{Math.round(j.percent)}%</b></div><div className="torrent-progress"><i style={{width:`${Math.min(100,j.percent)}%`}}/></div><div className="torrent-stats"><span>↓ {rate(j.download_rate)}</span><span>↑ {rate(j.upload_rate)}</span><span>{j.peers} peers</span><span>{bytes(j.downloaded_bytes)} / {bytes(j.total_bytes)}</span><span>Uploaded {bytes(j.uploaded_bytes)} · ratio {ratio.toFixed(2)}</span><span>Imported {j.imported_count}/{j.total_audio}</span></div>{j.status==="seeding"&&<div className="seed-clock"><strong>{duration(left)} remaining</strong><span>Seeding since {date(j.completed_at)} · scheduled through {date(j.seed_until)}</span></div>}{j.error&&<p className="form-error">{j.error}</p>}<div className="torrent-foot"><small>{j.status==='expired'?`14-day seed period ended ${date(j.seed_until)}`:j.status!=="seeding"?`Updated ${date(j.updated_at)}`:"Original files remain available to peers"}</small><div>{(j.status==='seeding'||j.status==='expired')&&<button onClick={()=>void api.extendTorrent(j.id).then(load)}>Seed 14 more days</button>}{j.status==='expired'&&<button className="danger" onClick={()=>void remove(j)}><Trash2/> Delete original</button>}</div></div></article>})}{!jobs.length&&<p>No torrent jobs yet.</p>}</section>
 </Page>
}
