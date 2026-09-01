import {
  Plus,
  Power,
  Radio as RadioIcon,
  RefreshCw,
  RotateCcw,
  Save,
  Trash2,
  Download,
} from "lucide-react";
import { useEffect, useState } from "react";
import { DataControls } from "../components/DataControls";
import { api, type AudioDevice, type AudioOutput, type AudioOutputUpdate, type Backup, type Station, type UpdateRelease, type UpdateStatus } from "../lib/api";
import { type SortOption, useDataView } from "../lib/useDataView";
import { useLiveEvents } from "../lib/useLiveEvents";
import { Page } from "./Dashboard";

const text = (a: string, b: string) =>
  a.localeCompare(b, undefined, { sensitivity: "base" });
const stationSorts: SortOption<Station>[] = [
  { value: "name", label: "Name", compare: (a, b) => text(a.name, b.name) },
  { value: "genre", label: "Genre", compare: (a, b) => text(a.genre, b.genre) },
  { value: "market", label: "Market", compare: (a, b) => text(a.market, b.market) },
  { value: "type", label: "Station type", compare: (a, b) => text(a.station_type, b.station_type) },
  { value: "format", label: "Format", compare: (a, b) => text(a.format, b.format) },
  {
    value: "favorite",
    label: "Favorite",
    compare: (a, b) => Number(b.favorite) - Number(a.favorite),
  },
];
const emptyStation: Station = {
  id: 0,
  name: "",
  stream_url: "",
  genre: "",
  favorite: false,
  call_sign: "", frequency: "", city: "", region: "", country: "US", market: "", station_type: "", format: "", description: "", website_url: "", enabled: true,
};
export function Radio() {
  const [items, setItems] = useState<Station[]>([]),
    [draft, setDraft] = useState<Station>(),
    [error, setError] = useState(""),
    [market, setMarket] = useState(""),
    [stationType, setStationType] = useState(""),
    [genre, setGenre] = useState("");
  const filtered = items.filter(x => (!market || x.market === market) && (!stationType || x.station_type === stationType) && (!genre || x.genre === genre));
  const view = useDataView(
    filtered,
    (x) => `${x.name} ${x.call_sign} ${x.frequency} ${x.genre} ${x.format} ${x.station_type} ${x.city} ${x.region} ${x.market} ${x.description}`,
    stationSorts,
  );
  const load = () =>
    api
      .radio()
      .then(setItems)
      .catch((e) => setError(String(e)));
  useEffect(() => {
    void load();
  }, []);
  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!draft) return;
    try {
      if (draft.id) await api.updateRadio(draft);
      else {
        const { id: _, ...body } = draft;
        await api.createRadio(body);
      }
      setDraft(undefined);
      await load();
    } catch (x) {
      setError(String(x));
    }
  };
  const remove = async (x: Station) => {
    if (!confirm(`Delete station “${x.name}”?`)) return;
    try {
      await api.deleteRadio(x.id);
      setDraft(undefined);
      await load();
    } catch (e) {
      setError(String(e));
    }
  };
  return (
    <Page
      title="Radio"
      sub="Manage internet radio stations and test streams."
      actions={
        <button
          className="primary"
          onClick={() => setDraft({ ...emptyStation })}
        >
          <Plus /> Add station
        </button>
      }
    >
      {error && <p className="form-error">{error}</p>}
      <div className="radio-filters"><select aria-label="Filter by market" value={market} onChange={e=>setMarket(e.target.value)}><option value="">All markets</option>{[...new Set(items.map(x=>x.market).filter(Boolean))].sort().map(x=><option key={x}>{x}</option>)}</select><select aria-label="Filter by station type" value={stationType} onChange={e=>setStationType(e.target.value)}><option value="">All station types</option>{[...new Set(items.map(x=>x.station_type).filter(Boolean))].sort().map(x=><option key={x}>{x}</option>)}</select><select aria-label="Filter by genre" value={genre} onChange={e=>setGenre(e.target.value)}><option value="">All genres</option>{[...new Set(items.map(x=>x.genre).filter(Boolean))].sort().map(x=><option key={x}>{x}</option>)}</select></div>
      <DataControls view={view} sorts={stationSorts} label="stations" />
      <div className="cards-3">
        {view.visible.map((x, i) => (
          <div className="collection-card station" key={x.id}>
            <div className={`fake-art square a${(i + 2) % 5}`} />
            <span className="admin-live">LIVE</span>
            <strong>{x.name}</strong>
            <span>{[x.city&&`${x.city}, ${x.region}`,x.station_type,x.format||x.genre].filter(Boolean).join(" · ")}</span>
            <div className="form-actions">
              <button onClick={() => void api.testRadio(x.id)}>
                <RadioIcon /> Test
              </button>
              <button onClick={() => setDraft({ ...x })}>Edit</button>
            </div>
          </div>
        ))}
        {view.total === 0 && <p>No matching radio stations.</p>}
      </div>
      {draft && (
        <form className="panel edit-form" onSubmit={(e) => void save(e)}>
          <div className="panel-title">
            <div>
              <span>STATION</span>
              <h2>{draft.id ? "Edit station" : "New station"}</h2>
            </div>
            <button type="button" onClick={() => setDraft(undefined)}>
              Close
            </button>
          </div>
          <div className="metadata-grid">
            <label>
              Name
              <input
                required
                value={draft.name}
                onChange={(e) => setDraft({ ...draft, name: e.target.value })}
              />
            </label>
            <label>
              Stream URL
              <input
                required
                type="url"
                value={draft.stream_url}
                onChange={(e) =>
                  setDraft({ ...draft, stream_url: e.target.value })
                }
              />
            </label>
            <label>
              Genre
              <input
                value={draft.genre}
                onChange={(e) => setDraft({ ...draft, genre: e.target.value })}
              />
            </label>
            {([['Call sign','call_sign'],['Frequency','frequency'],['City','city'],['State / region','region'],['Country','country'],['Market','market'],['Station type','station_type'],['Format','format'],['Website','website_url']] as const).map(([label,key])=><label key={key}>{label}<input value={draft[key]} onChange={e=>setDraft({...draft,[key]:e.target.value})}/></label>)}
            <label className="radio-description">Description<textarea value={draft.description} onChange={e=>setDraft({...draft,description:e.target.value})}/></label>
            <label className="check">
              <input
                type="checkbox"
                checked={draft.favorite}
                onChange={(e) =>
                  setDraft({ ...draft, favorite: e.target.checked })
                }
              />{" "}
              Favorite
            </label>
          </div>
          <div className="form-actions">
            {draft.id > 0 && (
              <button
                type="button"
                className="danger"
                onClick={() => void remove(draft)}
              >
                <Trash2 /> Delete
              </button>
            )}
            <button className="primary">
              <Save /> Save station
            </button>
          </div>
        </form>
      )}
    </Page>
  );
}

const backupSorts: SortOption<Backup>[] = [
  {
    value: "newest",
    label: "Newest",
    compare: (a, b) => text(b.created_at, a.created_at),
  },
  { value: "name", label: "Name", compare: (a, b) => text(a.name, b.name) },
  { value: "size", label: "Size", compare: (a, b) => a.size - b.size },
];
export function Settings() {
  const [backups, setBackups] = useState<Backup[]>([]),
    [busy, setBusy] = useState(false),
    [systemBusy, setSystemBusy] = useState(false),
    [systemMessage, setSystemMessage] = useState(""),
    [updateRelease,setUpdateRelease]=useState<UpdateRelease>(),
    [updateStatus,setUpdateStatus]=useState<UpdateStatus>(),
    [updateBusy,setUpdateBusy]=useState(false),
    [currentPassword,setCurrentPassword]=useState(""),
    [newPassword,setNewPassword]=useState(""),
    [accountMessage,setAccountMessage]=useState(""),
    [audioOutputs,setAudioOutputs]=useState<AudioOutput[]>([]),
    [audioDevices,setAudioDevices]=useState<AudioDevice[]>([]),
    [audioMessage,setAudioMessage]=useState(""),
    [tab,setTab]=useState(()=>location.hash.replace('#','')||'audio');
  const view = useDataView(
    backups,
    (b) => `${b.name} ${b.created_at}`,
    backupSorts,
    "newest",
  );
  const load = () => api.backups().then(setBackups);
  useEffect(() => {
    void load();
    void api.audioOutputs().then(setAudioOutputs).catch(()=>undefined);
    void api.audioDevices().then(x=>setAudioDevices(x.devices)).catch(()=>undefined);
    void api.updateStatus().then(setUpdateStatus).catch(()=>undefined);
  }, []);
  const create = async () => {
    setBusy(true);
    try {
      await api.createBackup();
      await load();
    } finally {
      setBusy(false);
    }
  };
  const recover = async () => {
    setSystemBusy(true);
    setSystemMessage("Recovering playback…");
    try {
      await api.recoverPlayer();
      setSystemMessage("Playback recovered and resumed.");
    } catch (e) {
      setSystemMessage(e instanceof Error ? e.message : "Recovery failed.");
    } finally {
      setSystemBusy(false);
    }
  };
  const restart = async () => {
    if (!window.confirm("Restart the player service and audio engine? Playback will pause briefly.")) return;
    setSystemBusy(true);
    setSystemMessage("Restarting player stack…");
    try {
      await api.restartSystem();
      const deadline = Date.now() + 20000;
      while (Date.now() < deadline) {
        await new Promise((resolve) => setTimeout(resolve, 750));
        try {
          await api.health();
          setSystemMessage("Player stack restarted.");
          return;
        } catch { /* service is still restarting */ }
      }
      setSystemMessage("Restart is taking longer than expected.");
    } catch (e) {
      setSystemMessage(e instanceof Error ? e.message : "Restart failed.");
    } finally {
      setSystemBusy(false);
    }
  };
  const changePassword=async()=>{setAccountMessage("");try{await api.changePassword(currentPassword,newPassword);setAccountMessage("Password changed. Sign in again with the new password.");window.dispatchEvent(new Event('ekkoplayer:auth-required'))}catch(e){setAccountMessage(e instanceof Error?e.message:"Password change failed")}};
  const logout=async(all=false)=>{try{if(all)await api.logoutAll();else await api.logout()}finally{window.dispatchEvent(new Event('ekkoplayer:auth-required'))}};
  const selectTab=(next:string)=>{setTab(next);history.replaceState(null,'',`#${next}`)};
  const patchLocal=(name:string,body:AudioOutputUpdate)=>setAudioOutputs(xs=>xs.map(x=>x.name===name?{...x,...body}:x));
  const updateOutput=async(name:string,body:AudioOutputUpdate)=>{try{await api.updateAudioOutput(name,body);patchLocal(name,body);setAudioMessage(Object.keys(body).some(k=>!['volume_trim','muted','delay_ms'].includes(k))?'Saved. Restart the player stack to activate hardware and processing changes.':'Audio output updated live.')}catch(e){setAudioMessage(e instanceof Error?e.message:'Audio update failed')}};
  const checkForUpdate=async()=>{setUpdateBusy(true);setSystemMessage("");try{const release=await api.checkUpdate();setUpdateRelease(release);setSystemMessage(release.available?`Version ${release.latest_version} is available.`:`Version ${release.current_version} is current.`)}catch(e){setSystemMessage(e instanceof Error?e.message:"Update check failed.")}finally{setUpdateBusy(false)}};
  const installUpdate=async()=>{if(!updateRelease?.available||!window.confirm(`Install ekkoPlayer ${updateRelease.latest_version}? Playback and Admin will be unavailable briefly.`))return;setUpdateBusy(true);setSystemMessage("Update queued. The appliance will verify, install, and restart automatically.");try{setUpdateStatus(await api.applyUpdate());const deadline=Date.now()+180000;while(Date.now()<deadline){await new Promise(resolve=>setTimeout(resolve,2000));try{const status=await api.updateStatus();setUpdateStatus(status);setSystemMessage(status.message||status.state);if(status.state==='complete'||status.state==='failed')break}catch{/* expected while the service restarts */}}}catch(e){setSystemMessage(e instanceof Error?e.message:"Update failed.")}finally{setUpdateBusy(false)}};
  return (
    <Page
      title="Settings"
      sub="Audio, system, security, and backup configuration."
      actions={tab==='backups'?
        <button
          className="primary"
          disabled={busy}
          onClick={() => void create()}
        >
          <Save /> Create backup
        </button>
      :undefined}
    >
      <div className="settings-tabs" role="tablist" aria-label="Settings sections">
        {['audio','system','security','backups'].map(x=><button key={x} role="tab" aria-selected={tab===x} className={tab===x?'active':''} onClick={()=>selectTab(x)}>{x[0].toUpperCase()+x.slice(1)}</button>)}
      </div>
      {tab==='audio'&&<div className="audio-settings">
        {audioMessage&&<div className="system-message" role="status">{audioMessage}</div>}
        {audioOutputs.map(o=>{
          const agc=o.filter||'';
          return <section className="panel form-panel audio-output-editor" key={o.name}>
            <div className="panel-title"><div><h2>{o.name}{o.primary?' · Primary':''}</h2><span>{o.device}</span></div><span className={o.online?'live-dot':'form-error'}>{o.online?'ONLINE':'OFFLINE'}</span></div>
            <div className="audio-control-grid">
              <label>Audio card / output<select value={o.device} onChange={e=>patchLocal(o.name,{device:e.target.value})}>{!audioDevices.some(d=>d.id===o.device)&&<option value={o.device}>{o.device} (configured)</option>}{audioDevices.map(d=><option key={d.id} value={d.id}>{d.label} — {d.id}</option>)}</select><small>Only outputs discovered by mpv on this appliance are listed.</small></label>
              <label>Automatic gain control<select value={agc} onChange={e=>patchLocal(o.name,{filter:e.target.value})}><option value="">Off — preserve source dynamics</option><option value="lavfi=[dynaudnorm=f=1000:g=7:p=0.90]">Gentle — albums and consistent libraries</option><option value="lavfi=[dynaudnorm=f=500:g=15:p=0.95]">Balanced — mixed music library</option><option value="lavfi=[dynaudnorm=f=250:g=25:p=0.92]">Strong — radio-style leveling</option></select><small>AGC evens out quiet and loud tracks. Balanced is the recommended general-purpose setting.</small></label>
              <label>Channel layout<select value={o.channels||''} onChange={e=>patchLocal(o.name,{channels:e.target.value})}><option value="">Automatic</option><option value="stereo">Stereo</option><option value="mono">Mono</option><option value="5.1">5.1 surround</option><option value="7.1">7.1 surround</option></select></label>
              <label>Sample rate<select value={o.sample_rate||0} onChange={e=>patchLocal(o.name,{sample_rate:Number(e.target.value)})}><option value={0}>Automatic</option><option value={44100}>44.1 kHz</option><option value={48000}>48 kHz</option><option value={96000}>96 kHz</option><option value={192000}>192 kHz</option></select></label>
              <label>Sample format<select value={o.format||''} onChange={e=>patchLocal(o.name,{format:e.target.value})}><option value="">Automatic</option><option value="s16">16-bit integer</option><option value="s24">24-bit integer</option><option value="s32">32-bit integer</option><option value="float">32-bit float</option></select></label>
              <label>Audio buffer (ms)<input type="number" min="20" max="5000" value={o.buffer_ms} onChange={e=>patchLocal(o.name,{buffer_ms:Number(e.target.value)})}/><small>Lower responds faster; higher resists dropouts. Start at 100 ms.</small></label>
              <label>Latency correction (ms)<input type="number" min="-5000" max="5000" value={o.delay_ms} onChange={e=>patchLocal(o.name,{delay_ms:Number(e.target.value)})}/><small>Positive values delay this zone to align it with other outputs.</small></label>
              <label>Drift tolerance (ms)<input type="number" min="1" max="1000" value={o.drift_correction_ms||40} onChange={e=>patchLocal(o.name,{drift_correction_ms:Number(e.target.value)})}/><small>Mirrored zones are resynchronized when they exceed this difference.</small></label>
            </div>
            <div className="audio-live-controls">
              <label>Output trim: {o.volume_trim>0?'+':''}{o.volume_trim} dB<input type="range" min="-100" max="100" value={o.volume_trim} onChange={e=>patchLocal(o.name,{volume_trim:Number(e.target.value)})} onPointerUp={e=>void updateOutput(o.name,{volume_trim:Number((e.target as HTMLInputElement).value)})}/></label>
              <label className="check"><input type="checkbox" checked={o.muted} onChange={e=>void updateOutput(o.name,{muted:e.target.checked})}/> Mute output</label>
              <label className="check"><input type="checkbox" checked={o.exclusive} onChange={e=>patchLocal(o.name,{exclusive:e.target.checked})}/> Exclusive hardware access</label>
            </div>
            <div className="audio-editor-footer"><small>Effective volume {o.effective_volume} · measured drift {o.drift_ms} ms</small><button className="primary" onClick={()=>void updateOutput(o.name,{device:o.device,buffer_ms:o.buffer_ms,channels:o.channels||'',sample_rate:o.sample_rate||0,format:o.format||'',exclusive:o.exclusive,filter:o.filter||'',drift_correction_ms:o.drift_correction_ms||40,delay_ms:o.delay_ms})}><Save/> Save audio configuration</button></div>
            {o.error&&<p className="form-error">{o.error}</p>}
          </section>})}
        {!audioOutputs.length&&<section className="panel"><p>No audio outputs are configured.</p></section>}
        <section className="panel audio-help"><h2>Applying audio changes</h2><p>Volume trim, mute, and latency correction apply live. Device, AGC, format, channels, buffer, and exclusive-mode changes are validated and activated when the player stack restarts.</p><button disabled={systemBusy} onClick={()=>void restart()}><Power/> Restart and apply</button></section>
      </div>}
      {tab==='system'&&<div className="settings-grid">
        <section className="panel form-panel">
          <h2>Player recovery</h2>
          <p>Recover reloads the current track at its saved position. Restart recreates the backend and audio engine.</p>
          <div className="system-buttons">
            <button disabled={systemBusy} onClick={() => void recover()}><RotateCcw /> Recover playback</button>
            <button className="danger-action" disabled={systemBusy} onClick={() => void restart()}><Power /> Restart player stack</button>
          </div>
          {systemMessage && <div className="system-message" role="status">{systemMessage}</div>}
        </section>
        <section className="panel form-panel system-panel">
          <h2 id="system-updates">System updates</h2>
          <p>Updates come from the official GitHub release, are SHA-256 verified, backed up, health checked, and rolled back automatically if startup fails.</p>
          <div className="update-version"><strong>Installed: {updateStatus?.current_version||updateRelease?.current_version||'checking…'}</strong>{updateRelease&&<span>Latest: {updateRelease.latest_version}</span>}{updateStatus?.target_version&&updateStatus.state!=='idle'&&<span>Status: {updateStatus.state} · {updateStatus.target_version}</span>}</div>
          <div className="system-buttons"><button disabled={updateBusy} onClick={()=>void checkForUpdate()}><RotateCcw/> Check for updates</button>{updateRelease?.available&&<button className="primary" disabled={updateBusy} onClick={()=>void installUpdate()}><Download/> Install {updateRelease.latest_version}</button>}</div>
        </section>
      </div>}
      {tab==='security'&&<div className="settings-grid"><section className="panel form-panel"><h2>Administrator session</h2><p>Access tokens expire after 15 minutes and refresh automatically. Refresh sessions expire after 30 days.</p><label>Current password<input type="password" autoComplete="current-password" value={currentPassword} onChange={e=>setCurrentPassword(e.target.value)}/></label><label>New password<input type="password" minLength={12} autoComplete="new-password" value={newPassword} onChange={e=>setNewPassword(e.target.value)}/></label><div className="system-buttons"><button className="primary" disabled={!currentPassword||newPassword.length<12} onClick={()=>void changePassword()}>Change password</button><button onClick={()=>void logout()}>Log out</button><button className="danger-action" onClick={()=>void logout(true)}>Log out everywhere</button></div>{accountMessage&&<div className="system-message" role="status">{accountMessage}</div>}</section></div>}
      {tab==='backups'&&<div className="settings-grid">
        <section className="panel">
          <h2>Backups</h2>
          <DataControls view={view} sorts={backupSorts} label="backups" />
          <div className="activity">
            {view.visible.map((b) => (
              <div className="activity-row" key={b.name}>
                <i className="ok" />
                <div>
                  <strong>{b.name}</strong>
                  <span>
                    {(b.size / 1024 / 1024).toFixed(1)} MB ·{" "}
                    {new Date(b.created_at).toLocaleString()}
                  </span>
                </div>
                <a href={`/api/v1/admin/backups/${encodeURIComponent(b.name)}`}>
                  Download
                </a>
              </div>
            ))}
            {view.total === 0 && <p>No matching backups.</p>}
          </div>
        </section>
      </div>}
    </Page>
  );
}
