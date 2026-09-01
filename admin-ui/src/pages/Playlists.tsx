import { ArrowDown, ArrowUp, ImagePlus, Plus, Trash2 } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { DataControls } from "../components/DataControls";
import { api, type Playlist, type Song } from "../lib/api";
import { type SortOption, useDataView } from "../lib/useDataView";
import { Page } from "./Dashboard";
const text = (a: string, b: string) =>
  a.localeCompare(b, undefined, { sensitivity: "base" });
const songSorts: SortOption<Song>[] = [
  { value: "title", label: "Title", compare: (a, b) => text(a.title, b.title) },
  { value: "artist", label: "Artist", compare: (a, b) => text(a.artist, b.artist) },
  { value: "album", label: "Album", compare: (a, b) => text(a.album, b.album) },
  { value: "duration", label: "Time", compare: (a, b) => a.duration_ms - b.duration_ms },
  { value: "size", label: "Size", compare: (a, b) => a.file_size - b.file_size },
  { value: "genre", label: "Genre", compare: (a, b) => text(a.genre || "", b.genre || "") },
];
const playlistSorts: SortOption<Playlist>[] = [
  { value: "name", label: "Name", compare: (a, b) => text(a.name, b.name) },
  { value: "songs", label: "Song count", compare: (a, b) => a.song_count - b.song_count },
  { value: "updated", label: "Updated", compare: (a, b) => text(a.updated_at, b.updated_at) },
];
export function Playlists() {
  const [items, setItems] = useState<Playlist[]>([]),
    [active, setActive] = useState<Playlist>(),
    [tracks, setTracks] = useState<Song[]>([]),
    [library, setLibrary] = useState<Song[]>([]),
    [chosen, setChosen] = useState<Set<number>>(new Set()),
    [available, setAvailable] = useState<Set<number>>(new Set()),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const artworkInput = useRef<HTMLInputElement>(null);
  const load = async () => {
    try {
      setItems(await api.playlists());
      setLibrary(await api.songs());
    } catch (e) {
      setError(String(e));
    }
  };
  useEffect(() => {
    void load();
  }, []);
  useEffect(() => {
    if (active) void api.playlistSongs(active.id).then(setTracks);
  }, [active]);
  const playlistView = useDataView(items, p => `${p.name} ${p.song_count}`, playlistSorts);
  const trackView = useDataView(tracks, s => `${s.title} ${s.artist} ${s.album} ${s.genre || ""}`, songSorts);
  const addView = useDataView(library.filter(s => !tracks.some(t => t.id === s.id)), s => `${s.title} ${s.artist} ${s.album} ${s.genre || ""}`, songSorts);
  const create = async () => {
    const name = prompt("Playlist name")?.trim();
    if (name) {
      await api.createPlaylist(name);
      await load();
    }
  };
  const add = async () => {
    if (!active || !available.size) return;
    setBusy(true);
    try {
      for (const id of available) await api.addPlaylistSong(active.id, id);
      setTracks(await api.playlistSongs(active.id));
      setAvailable(new Set());
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };
  const remove = async () => {
    if (!active || !chosen.size) return;
    setBusy(true);
    try {
      for (const id of chosen) await api.removePlaylistSong(active.id, id);
      setTracks(await api.playlistSongs(active.id));
      setChosen(new Set());
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };
  const move = async (direction: -1 | 1) => {
    if (!active || !chosen.size) return;
    const next = [...tracks];
    const range = direction < 0 ? [...next.keys()] : [...next.keys()].reverse();
    for (const i of range) {
      const j = i + direction;
      if (
        chosen.has(next[i].id) &&
        j >= 0 &&
        j < next.length &&
        !chosen.has(next[j].id)
      )
        [next[i], next[j]] = [next[j], next[i]];
    }
    setTracks(next);
    try {
      setTracks(
        await api.reorderPlaylist(
          active.id,
          next.map((s) => s.id),
        ),
      );
    } catch (e) {
      setError(String(e));
      setTracks(await api.playlistSongs(active.id));
    }
  };
  const toggle = (
    set: (x: Set<number>) => void,
    current: Set<number>,
    id: number,
  ) => {
    const next = new Set(current);
    next.has(id) ? next.delete(id) : next.add(id);
    set(next);
  };
  const updateArtwork = async (file?: File) => {
    if (!active || !file) return;
    setBusy(true);
    try {
      const next = await api.playlistArtwork(active.id, file);
      setItems(next);
      setActive(next.find((p) => p.id === active.id));
    } catch (e) { setError(String(e)); } finally { setBusy(false); }
  };
  const resetArtwork = async () => {
    if (!active) return;
    const next = await api.resetPlaylistArtwork(active.id);
    setItems(next);
    setActive(next.find((p) => p.id === active.id));
  };
  return (
    <Page
      title="Playlists"
      sub="Search, sort and organize tracks in bulk."
      actions={
        <button className="primary" onClick={() => void create()}>
          <Plus /> New playlist
        </button>
      }
    >
      {error && <p className="form-error">{error}</p>}
      <DataControls view={playlistView} sorts={playlistSorts} label="playlists" />
      <div className="cards-3">
        {playlistView.visible.map((p) => (
          <button
            className="collection-card"
            key={p.id}
            onClick={() => {
              setActive(p);
              setChosen(new Set());
              setAvailable(new Set());
            }}
          >
            <PlaylistCover playlist={p} />
            <strong>{p.name}</strong>
            <span>{p.song_count} songs</span>
          </button>
        ))}
      </div>
      {active && (
        <section className="panel playlist-workspace">
          <div className="panel-title">
            <div className="playlist-editor-title">
              <PlaylistCover playlist={active} />
              <div>
              <span>PLAYLIST EDITOR</span>
              <h2>{active.name}</h2>
              </div>
            </div>
            <div className="playlist-art-actions"><input ref={artworkInput} hidden type="file" accept="image/*" onChange={(e)=>void updateArtwork(e.target.files?.[0])}/><button disabled={busy} onClick={()=>artworkInput.current?.click()}><ImagePlus/> Choose cover</button><button disabled={busy} onClick={()=>void resetArtwork()}>Use gradient</button><button
              className="danger"
              onClick={() =>
                void api.deletePlaylist(active.id).then(() => {
                  setActive(undefined);
                  return load();
                })
              }
            >
              <Trash2 /> Delete playlist
            </button>
            </div>
          </div>
          <div className="playlist-columns">
            <div>
              <div className="editor-toolbar">
                <strong>In playlist ({tracks.length})</strong>
                <button
                  disabled={!chosen.size || busy}
                  onClick={() => void move(-1)}
                >
                  <ArrowUp /> Move up
                </button>
                <button
                  disabled={!chosen.size || busy}
                  onClick={() => void move(1)}
                >
                  <ArrowDown /> Move down
                </button>
                <button
                  className="danger"
                  disabled={!chosen.size || busy}
                  onClick={() => void remove()}
                >
                  <Trash2 /> Remove
                </button>
              </div>
              <DataControls view={trackView} sorts={songSorts} label="playlist songs" />
              <div className="pick-list">
                {trackView.visible.map((s) => (
                  <label key={s.id}>
                    <input
                      type="checkbox"
                      checked={chosen.has(s.id)}
                      onChange={() => toggle(setChosen, chosen, s.id)}
                    />
                    <span>
                      <strong>
                        {tracks.findIndex((track) => track.id === s.id) + 1}. {s.title}
                      </strong>
                      <small>
                        {s.artist} · {s.album} ·{" "}
                        {Math.round(s.duration_ms / 60000)} min ·{" "}
                        {(s.file_size / 1024 / 1024).toFixed(1)} MB
                      </small>
                    </span>
                  </label>
                ))}
              </div>
            </div>
            <div>
              <div className="editor-toolbar">
                <button
                  className="primary"
                  disabled={!available.size || busy}
                  onClick={() => void add()}
                >
                  <Plus /> Add {available.size || ""}
                </button>
              </div>
              <DataControls view={addView} sorts={songSorts} label="songs to add" />
              <div className="pick-list">
                {addView.visible.map((s) => (
                  <label key={s.id}>
                    <input
                      type="checkbox"
                      checked={available.has(s.id)}
                      onChange={() => toggle(setAvailable, available, s.id)}
                    />
                    <span>
                      <strong>{s.title}</strong>
                      <small>
                        {s.artist} · {s.album} · {s.genre || "No genre"} ·{" "}
                        {(s.file_size / 1024 / 1024).toFixed(1)} MB
                      </small>
                    </span>
                  </label>
                ))}
              </div>
            </div>
          </div>
        </section>
      )}
    </Page>
  );
}

const gradients:Record<string,string>={aurora:'linear-gradient(135deg,#064e3b,#059669 48%,#22d3ee)',cobalt:'linear-gradient(135deg,#172554,#2563eb 52%,#60a5fa)',sunset:'linear-gradient(135deg,#7c2d12,#ea580c 48%,#fbbf24)',orchid:'linear-gradient(135deg,#4c1d95,#a21caf 52%,#f472b6)',forest:'linear-gradient(135deg,#052e16,#15803d 52%,#84cc16)',ember:'linear-gradient(135deg,#450a0a,#dc2626 52%,#fb7185)',lagoon:'linear-gradient(135deg,#083344,#0891b2 52%,#2dd4bf)',berry:'linear-gradient(135deg,#4c0519,#be185d 50%,#a78bfa)'};
function PlaylistCover({playlist}:{playlist:Playlist}){const generated=playlist.artwork?.startsWith('playlist-gradient:'),key=generated?playlist.artwork!.slice(18):'';return <div className="playlist-cover" style={generated?{background:gradients[key]||gradients.cobalt}:undefined}>{playlist.artwork&&!generated&&<img src={playlist.artwork} alt=""/>}{(!playlist.artwork||generated)&&<strong>{playlist.name}</strong>}</div>}
