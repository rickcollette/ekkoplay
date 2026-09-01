import { Edit3, Folder, FolderPlus, Trash2, Upload } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { DataControls } from "../components/DataControls";
import { UploadProgress } from "../components/UploadProgress";
import {
  api,
  type FolderInfo,
  type Playlist,
  type Song,
  type UploadProgress as Progress,
} from "../lib/api";
import { type SortOption, useDataView } from "../lib/useDataView";
import { useLiveEvents } from "../lib/useLiveEvents";
import { Page } from "./Dashboard";
const join = (a: string, b: string) => (a ? `${a}/${b}` : b);
const parent = (path: string) =>
  path.includes("/") ? path.slice(0, path.lastIndexOf("/")) : "";
const basename = (path: string) => path.split("/").pop() || path;
type Entry =
  | { kind: "folder"; folder: FolderInfo }
  | { kind: "song"; song: Song };
const entrySorts: SortOption<Entry>[] = [
  {
    value: "name",
    label: "Name",
    compare: (a, b) =>
      (a.kind === "folder"
        ? a.folder.name
        : a.song.original_filename || a.song.title
      ).localeCompare(
        b.kind === "folder"
          ? b.folder.name
          : b.song.original_filename || b.song.title,
        undefined,
        { sensitivity: "base" },
      ),
  },
  {
    value: "type",
    label: "Type",
    compare: (a, b) => a.kind.localeCompare(b.kind),
  },
  {
    value: "artist",
    label: "Artist",
    compare: (a, b) =>
      (a.kind === "song" ? a.song.artist : "").localeCompare(
        b.kind === "song" ? b.song.artist : "",
        undefined,
        { sensitivity: "base" },
      ),
  },
  {
    value: "size",
    label: "Size",
    compare: (a, b) =>
      (a.kind === "song" ? a.song.file_size : 0) -
      (b.kind === "song" ? b.song.file_size : 0),
  },
];
export function Folders() {
  const [searchParams, setSearchParams] = useSearchParams();
  const open = searchParams.get("path") || "";
  const setOpen = (path: string, replace = false) => {
    const next = new URLSearchParams(searchParams);
    if (path) next.set("path", path);
    else next.delete("path");
    setSearchParams(next, { replace });
  };
  const [songs, setSongs] = useState<Song[]>([]),
    [folders, setFolders] = useState<FolderInfo[]>([]),
    [playlists, setPlaylists] = useState<Playlist[]>([]),
    [selected, setSelected] = useState<Set<number>>(new Set()),
    [selectedFolders, setSelectedFolders] = useState<Set<string>>(new Set()),
    [selectionMode, setSelectionMode] = useState(false),
    [message, setMessage] = useState(""),
    [editing, setEditing] = useState<Song>(),
    [dragOver, setDragOver] = useState(""),
    [uploading, setUploading] = useState<{
      progress: Progress;
      count: number;
    }>();
  const artworkInput = useRef<HTMLInputElement>(null);
  const load = async () => {
    try {
      const [s, f, p] = await Promise.all([
        api.songs(),
        api.folders(),
        api.playlists(),
      ]);
      setSongs(s);
      setFolders(f);
      setPlaylists(p);
      setMessage("");
    } catch (e) {
      setMessage(String(e));
    }
  };
  useEffect(() => {
    void load();
  }, []);
  useLiveEvents(() => void load(), ["library.changed"], 750);
  const folderFor = (song: Song) => {
    const dir = (song.file_path || "")
      .replaceAll("\\", "/")
      .split("/")
      .slice(0, -1)
      .join("/");
    return (
      folders
        .map((f) => f.path)
        .filter((path) => path && dir.endsWith("/" + path))
        .sort((a, b) => b.length - a.length)[0] || ""
    );
  };
  const rawChildren = useMemo(
    () => folders.filter((f) => parent(f.path) === open && f.path !== open),
    [folders, open],
  );
  const rawTracks = useMemo(
    () => songs.filter((s) => folderFor(s) === open),
    [songs, folders, open],
  );
  const folderCounts = useMemo(() => {
    const counts = new Map<string, number>();
    for (const song of songs) {
      let path = folderFor(song);
      while (path) {
        counts.set(path, (counts.get(path) || 0) + 1);
        path = parent(path);
      }
    }
    return counts;
  }, [songs, folders]);
  const entries = useMemo<Entry[]>(
    () => [
      ...rawChildren.map((folder) => ({ kind: "folder" as const, folder })),
      ...rawTracks.map((song) => ({ kind: "song" as const, song })),
    ],
    [rawChildren, rawTracks],
  );
  const view = useDataView(
    entries,
    (e) =>
      e.kind === "folder"
        ? `${e.folder.name} ${e.folder.path}`
        : `${e.song.original_filename} ${e.song.title} ${e.song.artist} ${e.song.album}`,
    entrySorts,
  );
  const visibleFolders = view.visible
      .filter(
        (e): e is Extract<Entry, { kind: "folder" }> => e.kind === "folder",
      )
      .map((e) => e.folder),
    visibleTracks = view.visible
      .filter((e): e is Extract<Entry, { kind: "song" }> => e.kind === "song")
      .map((e) => e.song);
  const children = visibleFolders,
    tracks = visibleTracks;
  const create = async () => {
    const name = prompt("New folder name")?.trim();
    if (!name) return;
    try {
      setFolders(await api.createFolder(join(open, name)));
    } catch (e) {
      setMessage(String(e));
    }
  };
  const rename = async (path: string) => {
    const name = prompt("Rename folder", basename(path))?.trim();
    if (!name || name === basename(path)) return;
    try {
      setFolders(await api.moveFolder(path, join(parent(path), name)));
      if (open === path) setOpen(join(parent(path), name), true);
      await load();
    } catch (e) {
      setMessage(String(e));
    }
  };
  const remove = async (path: string) => {
    if (
      !confirm(
        `Delete folder “${basename(path)}” and everything inside it? Songs and files will be retained in recovery storage.`,
      )
    )
      return;
    try {
      await api.deleteFolder(path);
      if (open === path) setOpen(parent(path), true);
      await load();
    } catch (e) {
      setMessage(String(e));
    }
  };
  const moveFolder = async (path: string, target: string) => {
    if (path === target || parent(path) === target) return;
    try {
      await api.moveFolder(path, join(target, basename(path)));
      await load();
    } catch (e) {
      setMessage(String(e));
    }
  };
  const moveSongs = async (ids: number[], target: string) => {
    if (!ids.length) return;
    try {
      await api.moveSongs(ids, target);
      setSelected(new Set());
      await load();
    } catch (e) {
      setMessage(String(e));
    }
  };
  const upload = async (files: File[], target: string) => {
    if (!files.length) return;
    setUploading({
      count: files.length,
      progress: {
        loaded: 0,
        total: files.reduce((n, f) => n + f.size, 0),
        percent: 0,
      },
    });
    try {
      await api.upload(
        files,
        (progress) => setUploading({ count: files.length, progress }),
        target,
      );
      setMessage(
        `${files.length} ${files.length === 1 ? "file" : "files"} queued for ${target || "Library root"}.`,
      );
    } catch (e) {
      setMessage(String(e));
    } finally {
      setUploading(undefined);
    }
  };
  const drop = async (e: React.DragEvent, target: string) => {
    e.preventDefault();
    setDragOver("");
    const folder = e.dataTransfer.getData("application/x-ekkoplayer-folder");
    const song = e.dataTransfer.getData("application/x-ekkoplayer-songs");
    if (folder) {
      await moveFolder(folder, target);
      return;
    }
    if (song) {
      await moveSongs(JSON.parse(song), target);
      return;
    }
    await upload([...e.dataTransfer.files], target);
  };
  const addPlaylist = async (id: number) => {
    if (!id) return;
    const ids = new Set<number>(selected);
    if (selectionMode) {
      for (const song of songs) {
        const path = folderFor(song);
        if (
          [...selectedFolders].some(
            (folder) => path === folder || path.startsWith(folder + "/"),
          )
        )
          ids.add(song.id);
      }
    } else {
      for (const song of songs) {
        const path = folderFor(song);
        if (!open || path === open || path.startsWith(open + "/"))
          ids.add(song.id);
      }
    }
    if (!ids.size) {
      setMessage("No files are selected.");
      return;
    }
    try {
      await api.addPlaylistSongs(id, [...ids]);
      setSelected(new Set());
      setSelectedFolders(new Set());
      setSelectionMode(false);
      setMessage(`Added ${ids.size} tracks to the playlist.`);
    } catch (e) {
      setMessage(String(e));
    }
  };
  const toggle = (id: number) => {
    setSelectionMode(true);
    setSelected((current) => {
      const next = new Set(current);
      next.has(id) ? next.delete(id) : next.add(id);
      return next;
    });
  };
  const toggleFolder = (path: string) => {
    setSelectionMode(true);
    setSelectedFolders((current) => {
      const next = new Set(current);
      next.has(path) ? next.delete(path) : next.add(path);
      return next;
    });
  };
  const selectionCount = selected.size + selectedFolders.size;
  const bulkMove = async (target: string) => {
    if (!target && target !== "") return;
    try {
      if (selected.size) await api.moveSongs([...selected], target);
      const roots = [...selectedFolders].filter(
        (path) =>
          ![...selectedFolders].some(
            (other) => path !== other && path.startsWith(other + "/"),
          ),
      );
      for (const path of roots)
        await api.moveFolder(path, join(target, basename(path)));
      setSelected(new Set());
      setSelectedFolders(new Set());
      await load();
    } catch (e) {
      setMessage(String(e));
    }
  };
  const bulkDelete = async () => {
    if (
      !selectionCount ||
      !confirm(
        `Delete ${selected.size} selected files and ${selectedFolders.size} selected folders? Files retain recovery copies; folders must be empty after file deletion.`,
      )
    )
      return;
    try {
      for (const id of selected) await api.songDelete(id);
      const remaining = await api.folders();
      const existing = new Set(remaining.map((f) => f.path));
      for (const path of [...selectedFolders].sort(
        (a, b) => b.length - a.length,
      ))
        if (existing.has(path)) await api.deleteFolder(path);
      setSelected(new Set());
      setSelectedFolders(new Set());
      await load();
    } catch (e) {
      setMessage(String(e));
      await load();
    }
  };
  const renameFile = async (song: Song) => {
    const current =
      (song.file_path || song.original_filename).split("/").pop() ||
      song.original_filename;
    const name = prompt("Rename file", current)?.trim();
    if (!name || name === current) return;
    try {
      await api.renameFile(song.id, name);
      await load();
    } catch (e) {
      setMessage(String(e));
    }
  };
  const saveMetadata = async () => {
    if (!editing) return;
    try {
      const updated = await api.songUpdate(editing.id, {
        title: editing.title.trim(),
        artist: editing.artist.trim(),
        album: editing.album.trim(),
        genre: editing.genre.trim(),
        year: Number(editing.year) || 0,
        track_number: Number(editing.track_number) || 0,
      });
      const art = artworkInput.current?.files?.[0];
      if (art) await api.songArtwork(updated.id, art);
      setEditing(undefined);
      setMessage("Metadata saved and locked against automatic replacement.");
      await load();
    } catch (e) {
      setMessage(String(e));
    }
  };
  const allSelected =
    visibleFolders.every((f) => selectedFolders.has(f.path)) &&
    visibleTracks.every((s) => selected.has(s.id)) &&
    view.visible.length > 0;
  const selectAll = () => {
    setSelectionMode(true);
    if (allSelected) {
      setSelected((current) => {
        const next = new Set(current);
        visibleTracks.forEach((s) => next.delete(s.id));
        return next;
      });
      setSelectedFolders((current) => {
        const next = new Set(current);
        visibleFolders.forEach((f) => next.delete(f.path));
        return next;
      });
    } else {
      setSelected(
        (current) => new Set([...current, ...visibleTracks.map((s) => s.id)]),
      );
      setSelectedFolders(
        (current) =>
          new Set([...current, ...visibleFolders.map((f) => f.path)]),
      );
    }
  };
  return (
    <Page
      title="Media Manager"
      sub="Manage the music library. Create folders, rename or move media, upload files, and organize items in bulk."
    >
      <div className="folder-command">
        <button disabled={!open} onClick={() => setOpen(parent(open))}>
          Up
        </button>
        <strong>Music / {open || "Library root"}</strong>
        <button onClick={() => void create()}>
          <FolderPlus /> New folder
        </button>
        <select
          aria-label="Add current folder to playlist"
          value=""
          onChange={(e) => void addPlaylist(Number(e.target.value))}
        >
          <option value="">Add folder to playlist…</option>
          {playlists.map((p) => (
            <option value={p.id} key={p.id}>
              {p.name}
            </option>
          ))}
        </select>
      </div>
      <DataControls view={view} sorts={entrySorts} label="folder items" />
      <div className="toolbar">
        <button onClick={selectAll}>
          {allSelected ? "Clear all" : "Select all"}
        </button>
      </div>
      {selectionCount > 0 && (
        <div className="folder-bulk">
          <strong>{selectionCount} selected</strong>
          <select
            aria-label="Move selected to folder"
            value=""
            onChange={(e) => {
              void bulkMove(e.target.value);
              e.target.value = "";
            }}
          >
            <option value="">Move to…</option>
            <option value=".">Library root</option>
            {folders
              .filter((f) => f.path && !selectedFolders.has(f.path))
              .map((f) => (
                <option key={f.path} value={f.path}>
                  {f.path}
                </option>
              ))}
          </select>
          <button className="danger" onClick={() => void bulkDelete()}>
            <Trash2 /> Delete selected
          </button>
          <button
            onClick={() => {
              setSelected(new Set());
              setSelectedFolders(new Set());
            }}
          >
            Clear
          </button>
        </div>
      )}
      {uploading && <UploadProgress {...uploading} />}{" "}
      {message && <p className="folder-message">{message}</p>}
      {editing && (
        <section className="card metadata-editor">
          <h3>Edit track metadata</h3>
          <p><strong>{editing.original_filename}</strong></p>
          <div className="form-grid">
            {(["title", "artist", "album", "genre"] as const).map((field) => (
              <label key={field}>
                {field[0].toUpperCase() + field.slice(1)}
                <input value={editing[field] || ""} onChange={(e) => setEditing({ ...editing, [field]: e.target.value })} />
              </label>
            ))}
            <label>Year<input type="number" value={editing.year || ""} onChange={(e) => setEditing({ ...editing, year: Number(e.target.value) })} /></label>
            <label>Track<input type="number" value={editing.track_number || ""} onChange={(e) => setEditing({ ...editing, track_number: Number(e.target.value) })} /></label>
            <label>Length<input disabled value={`${Math.floor(editing.duration_ms / 60000)}:${String(Math.floor(editing.duration_ms / 1000) % 60).padStart(2, "0")}`} /></label>
            <label>Album cover<input ref={artworkInput} type="file" accept="image/*" /></label>
          </div>
          <small>Source: {editing.metadata_source || "embedded/imported"} · confidence: {editing.metadata_confidence || 0}%</small>
          <div className="toolbar"><button onClick={() => void saveMetadata()}>Save metadata</button><button onClick={() => setEditing(undefined)}>Cancel</button></div>
        </section>
      )}
      <div
        className={`folder-drop-root ${dragOver === open ? "drag-over" : ""}`}
        onDragOver={(e) => {
          e.preventDefault();
          setDragOver(open);
        }}
        onDragLeave={() => setDragOver("")}
        onDrop={(e) => void drop(e, open)}
      >
        <Upload />
        <span>Drop audio files, songs, or folders here</span>
      </div>
      <div className="folder-browser">
        {children.map((f) => (
          <section
            key={f.path}
            className={`folder-node ${dragOver === f.path ? "drag-over" : ""}`}
            draggable
            onDragStart={(e) => {
              const paths = selectedFolders.has(f.path)
                ? [...selectedFolders]
                : [f.path];
              e.dataTransfer.setData(
                "application/x-ekkoplayer-folder",
                paths[0],
              );
              e.dataTransfer.effectAllowed = "move";
            }}
            onDragOver={(e) => {
              e.preventDefault();
              e.stopPropagation();
              setDragOver(f.path);
            }}
            onDrop={(e) => {
              e.stopPropagation();
              void drop(e, f.path);
            }}
          >
            <input
              aria-label={`Select folder ${f.name}`}
              type="checkbox"
              checked={selectedFolders.has(f.path)}
              onChange={() => toggleFolder(f.path)}
            />
            <button
              className="folder-open"
              aria-label={`Open folder ${f.name}`}
              onClick={() => {
                setOpen(f.path);
                setSelected(new Set());
                setSelectedFolders(new Set());
              }}
            >
              <Folder />
              <span>
                <strong>{f.name}</strong>
                <small>{folderCounts.get(f.path) || 0} tracks · {f.path}</small>
              </span>
            </button>
            <div>
              <button onClick={() => void rename(f.path)}>
                <Edit3 /> Rename
              </button>
              <button onClick={() => void remove(f.path)}>
                <Trash2 /> Delete
              </button>
            </div>
          </section>
        ))}
        {tracks.map((s) => (
          <div
            key={s.id}
            className="folder-song"
            draggable
            onDragStart={(e) => {
              const ids = selected.has(s.id) ? [...selected] : [s.id];
              e.dataTransfer.setData(
                "application/x-ekkoplayer-songs",
                JSON.stringify(ids),
              );
              e.dataTransfer.effectAllowed = "move";
            }}
          >
            <input
              aria-label={`Select file ${s.original_filename || s.title}`}
              type="checkbox"
              checked={selected.has(s.id)}
              onChange={() => toggle(s.id)}
            />
            <span>
              <strong>{s.original_filename || s.title}</strong>
              <small>
                {s.title} · {s.artist}
              </small>
            </span>
            <div>
              <button onClick={() => setEditing({ ...s })}>
                <Edit3 /> Metadata
              </button>
              <button onClick={() => void renameFile(s)}>
                <Edit3 /> Rename
              </button>
              <button onClick={() => void api.songDelete(s.id).then(load)}>
                <Trash2 /> Delete
              </button>
            </div>
          </div>
        ))}
      </div>
      {view.total === 0 && (
        <p>
          This folder is empty. Drop audio files here or create a subfolder.
        </p>
      )}
    </Page>
  );
}
