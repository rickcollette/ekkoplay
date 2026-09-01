package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"

	"ekkoplayer/internal/model"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type Store struct{ DB *sql.DB }

func Open(path string) (*Store, error) { return OpenWithDemo(path, false) }
func OpenWithDemo(path string, seed bool) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	pragmas := []string{"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=5000", "PRAGMA synchronous=NORMAL"}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, err
		}
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		return nil, err
	}
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		var applied int
		if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE name=?", e.Name()).Scan(&applied); err != nil {
			return nil, err
		}
		if applied > 0 {
			continue
		}
		b, err := migrationFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return nil, err
		}
		tx, err := db.Begin()
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(string(b)); err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("migration %s: %w", e.Name(), err)
		}
		if _, err := tx.Exec("INSERT INTO schema_migrations(name) VALUES(?)", e.Name()); err != nil {
			tx.Rollback()
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}
	s := &Store{DB: db}
	if seed {
		if err := s.seedDemo(context.Background()); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) seedDemo(ctx context.Context) error {
	var count int
	if err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM songs").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	artists := []string{"Night Geometry", "Static Avenue", "Glass Signals", "The Long Division", "Low Satellite"}
	for _, a := range artists {
		if _, err := tx.ExecContext(ctx, "INSERT INTO artists(name) VALUES(?)", a); err != nil {
			return err
		}
	}
	albums := []struct {
		title  string
		artist int
		year   int
		art    string
	}{
		{"Afterimage", 1, 2026, "gradient:violet"}, {"Nocturne Lines", 2, 2025, "gradient:blue"}, {"Cold Receiver", 3, 2026, "gradient:orange"}, {"Northbound", 4, 2024, "gradient:green"}, {"Signal Fires", 5, 2023, "gradient:red"},
	}
	for _, a := range albums {
		if _, err := tx.ExecContext(ctx, "INSERT INTO albums(title,artist_id,year,artwork) VALUES(?,?,?,?)", a.title, a.artist, a.year, a.art); err != nil {
			return err
		}
	}
	songs := []struct {
		title         string
		artist, album int
		year          int
		dur           int64
		art           string
	}{
		{"Midnight Circuit", 1, 1, 2026, 252000, "gradient:violet"}, {"Afterimage", 1, 1, 2026, 281000, "gradient:violet"}, {"Quiet Machines", 1, 1, 2026, 219000, "gradient:violet"},
		{"Streetlight Memory", 2, 2, 2025, 244000, "gradient:blue"}, {"Last Train Home", 2, 2, 2025, 273000, "gradient:blue"}, {"Neon Weather", 2, 2, 2025, 231000, "gradient:blue"},
		{"Cold Receiver", 3, 3, 2026, 265000, "gradient:orange"}, {"Open Channel", 3, 3, 2026, 202000, "gradient:orange"}, {"Dead Air", 3, 3, 2026, 295000, "gradient:orange"},
		{"Northbound", 4, 4, 2024, 312000, "gradient:green"}, {"Mile Marker", 4, 4, 2024, 228000, "gradient:green"}, {"No Exit", 4, 4, 2024, 246000, "gradient:green"},
		{"Signal Fires", 5, 5, 2023, 238000, "gradient:red"}, {"Low Orbit", 5, 5, 2023, 259000, "gradient:red"}, {"Beacon", 5, 5, 2023, 221000, "gradient:red"},
	}
	for i, x := range songs {
		path := fmt.Sprintf("/srv/ekkoplayer/music/demo/%02d-demo.flac", i+1)
		if _, err := tx.ExecContext(ctx, `INSERT INTO songs(title,artist_id,album_id,year,duration_ms,file_path,format,artwork,favorite) VALUES(?,?,?,?,?,?,?,?,?)`, x.title, x.artist, x.album, x.year, x.dur, path, "FLAC", x.art, boolInt(i%5 == 0)); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO playlists(name,artwork) VALUES('Late Night','gradient:violet'),('Driving','gradient:blue'),('Favorites','gradient:red')"); err != nil {
		return err
	}
	for i, song := range []int{1, 4, 7, 10, 13} {
		if _, err := tx.ExecContext(ctx, "INSERT INTO playlist_items(playlist_id,song_id,position) VALUES(1,?,?)", song, i); err != nil {
			return err
		}
	}
	for i, song := range []int{2, 5, 8, 11, 14} {
		if _, err := tx.ExecContext(ctx, "INSERT INTO playlist_items(playlist_id,song_id,position) VALUES(2,?,?)", song, i); err != nil {
			return err
		}
	}
	for i, song := range []int{1, 6, 11} {
		if _, err := tx.ExecContext(ctx, "INSERT INTO playlist_items(playlist_id,song_id,position) VALUES(3,?,?)", song, i); err != nil {
			return err
		}
	}
	stations := [][]any{{"Groove Current", "https://example.invalid/groove", "Ambient / Downtempo", "gradient:green", 1}, {"Night Transmission", "https://example.invalid/night", "Alternative", "gradient:violet", 1}, {"Deep Frequency", "https://example.invalid/deep", "Electronic", "gradient:blue", 0}}
	for _, r := range stations {
		if _, err := tx.ExecContext(ctx, "INSERT INTO radio_stations(name,stream_url,genre,artwork,favorite) VALUES(?,?,?,?,?)", r...); err != nil {
			return err
		}
	}
	for i, song := range []int{1, 4, 7, 10, 13} {
		if _, err := tx.ExecContext(ctx, "INSERT INTO queue_items(song_id,position) VALUES(?,?)", song, i); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE player_state SET track_id=1, status='paused' WHERE id=1"); err != nil {
		return err
	}
	return tx.Commit()
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

const songSelect = `SELECT s.id,s.title,a.name,al.title,al.id,a.id,s.year,s.duration_ms,s.file_path,s.format,COALESCE(NULLIF(s.artwork,''),al.artwork),s.favorite,s.track_number,s.disc_number,s.codec,s.bitrate,s.sample_rate,s.channels,s.file_size,s.sha256,s.original_filename,s.imported_at,s.genre,s.metadata_source,s.metadata_confidence,s.recording_mbid,s.release_group_mbid,s.release_mbid,s.metadata_locked FROM songs s JOIN artists a ON a.id=s.artist_id JOIN albums al ON al.id=s.album_id`

func scanSong(scanner interface{ Scan(...any) error }) (model.Song, error) {
	var x model.Song
	var fav, locked int
	err := scanner.Scan(&x.ID, &x.Title, &x.Artist, &x.Album, &x.AlbumID, &x.ArtistID, &x.Year, &x.DurationMS, &x.FilePath, &x.Format, &x.Artwork, &fav, &x.TrackNumber, &x.DiscNumber, &x.Codec, &x.Bitrate, &x.SampleRate, &x.Channels, &x.FileSize, &x.SHA256, &x.OriginalFilename, &x.ImportedAt, &x.Genre, &x.MetadataSource, &x.MetadataConfidence, &x.RecordingMBID, &x.ReleaseGroupMBID, &x.ReleaseMBID, &locked)
	x.Favorite = fav != 0
	x.MetadataLocked = locked != 0
	return x, err
}

func (s *Store) Songs(ctx context.Context, limit int) ([]model.Song, error) {
	q := songSelect + " ORDER BY s.title COLLATE NOCASE"
	args := []any{}
	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.Song, 0)
	for rows.Next() {
		x, e := scanSong(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Store) SongsPage(ctx context.Context, q string, page, pageSize int) (model.Page[model.Song], error) {
	return s.SongsPageSorted(ctx, q, page, pageSize, "title", false)
}
func (s *Store) SongsPageSorted(ctx context.Context, q string, page, pageSize int, sortKey string, descending bool) (model.Page[model.Song], error) {
	out := model.Page[model.Song]{Items: make([]model.Song, 0), Page: page, PageSize: pageSize}
	where := ""
	args := []any{}
	q = strings.TrimSpace(q)
	if q != "" {
		where = " WHERE lower(s.title) LIKE ? OR lower(a.name) LIKE ? OR lower(al.title) LIKE ?"
		like := "%" + strings.ToLower(q) + "%"
		args = []any{like, like, like}
	}
	if e := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM songs s JOIN artists a ON a.id=s.artist_id JOIN albums al ON al.id=s.album_id`+where, args...).Scan(&out.Total); e != nil {
		return out, e
	}
	queryArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	order := map[string]string{"title": "s.title COLLATE NOCASE", "artist": "a.name COLLATE NOCASE", "album": "al.title COLLATE NOCASE", "duration": "s.duration_ms", "size": "s.file_size", "imported": "s.imported_at"}[sortKey]
	if order == "" {
		order = "s.title COLLATE NOCASE"
	}
	if descending {
		order += " DESC"
	} else {
		order += " ASC"
	}
	rows, e := s.DB.QueryContext(ctx, songSelect+where+" ORDER BY "+order+",s.id LIMIT ? OFFSET ?", queryArgs...)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		x, e := scanSong(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, x)
	}
	return out, rows.Err()
}
func (s *Store) Song(ctx context.Context, id int64) (model.Song, error) {
	return scanSong(s.DB.QueryRowContext(ctx, songSelect+" WHERE s.id=?", id))
}
func (s *Store) RecentSongs(ctx context.Context, favorite bool) ([]model.Song, error) {
	q := songSelect
	if favorite {
		q += " WHERE s.favorite=1"
	}
	q += " ORDER BY s.id DESC LIMIT 8"
	rows, e := s.DB.QueryContext(ctx, q)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]model.Song, 0)
	for rows.Next() {
		x, e := scanSong(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Store) RecentlyPlayed(ctx context.Context) ([]model.Song, error) {
	rows, e := s.DB.QueryContext(ctx, songSelect+` JOIN (SELECT song_id,MAX(played_at) played FROM play_history GROUP BY song_id) h ON h.song_id=s.id ORDER BY h.played DESC LIMIT 8`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]model.Song, 0)
	for rows.Next() {
		x, e := scanSong(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) Albums(ctx context.Context) ([]model.Album, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT al.id,al.title,a.name,a.id,al.year,al.artwork,COUNT(s.id) FROM albums al JOIN artists a ON a.id=al.artist_id LEFT JOIN songs s ON s.album_id=al.id GROUP BY al.id ORDER BY al.title COLLATE NOCASE`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]model.Album, 0)
	for rows.Next() {
		var x model.Album
		if e := rows.Scan(&x.ID, &x.Title, &x.Artist, &x.ArtistID, &x.Year, &x.Artwork, &x.SongCount); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Store) Artists(ctx context.Context) ([]model.Artist, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT a.id,a.name,COUNT(DISTINCT al.id),COUNT(s.id),a.artwork FROM artists a LEFT JOIN albums al ON al.artist_id=a.id LEFT JOIN songs s ON s.artist_id=a.id GROUP BY a.id ORDER BY a.name COLLATE NOCASE`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]model.Artist, 0)
	for rows.Next() {
		var x model.Artist
		if e := rows.Scan(&x.ID, &x.Name, &x.AlbumCount, &x.SongCount, &x.Artwork); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Store) Playlists(ctx context.Context) ([]model.Playlist, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT p.id,p.name,COUNT(pi.id),p.artwork,p.updated_at FROM playlists p LEFT JOIN playlist_items pi ON pi.playlist_id=p.id GROUP BY p.id ORDER BY p.name COLLATE NOCASE`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]model.Playlist, 0)
	for rows.Next() {
		var x model.Playlist
		if e := rows.Scan(&x.ID, &x.Name, &x.SongCount, &x.Artwork, &x.UpdatedAt); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Store) Radio(ctx context.Context) ([]model.RadioStation, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT id,name,stream_url,genre,artwork,favorite,call_sign,frequency,city,region,country,market,station_type,format,description,website_url,enabled FROM radio_stations WHERE enabled=1 ORDER BY favorite DESC,name COLLATE NOCASE`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]model.RadioStation, 0)
	for rows.Next() {
		var x model.RadioStation
		var f, enabled int
		if e := rows.Scan(&x.ID, &x.Name, &x.StreamURL, &x.Genre, &x.Artwork, &f, &x.CallSign, &x.Frequency, &x.City, &x.Region, &x.Country, &x.Market, &x.StationType, &x.Format, &x.Description, &x.WebsiteURL, &enabled); e != nil {
			return nil, e
		}
		x.Favorite = f != 0
		x.Enabled = enabled != 0
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Store) RadioByID(ctx context.Context, id int64) (model.RadioStation, error) {
	var x model.RadioStation
	var f, enabled int
	e := s.DB.QueryRowContext(ctx, `SELECT id,name,stream_url,genre,artwork,favorite,call_sign,frequency,city,region,country,market,station_type,format,description,website_url,enabled FROM radio_stations WHERE id=?`, id).Scan(&x.ID, &x.Name, &x.StreamURL, &x.Genre, &x.Artwork, &f, &x.CallSign, &x.Frequency, &x.City, &x.Region, &x.Country, &x.Market, &x.StationType, &x.Format, &x.Description, &x.WebsiteURL, &enabled)
	x.Favorite = f != 0
	x.Enabled = enabled != 0
	return x, e
}
func (s *Store) SaveRadio(ctx context.Context, x model.RadioStation) (int64, error) {
	if strings.TrimSpace(x.Name) == "" || strings.TrimSpace(x.StreamURL) == "" {
		return 0, errors.New("name and stream_url are required")
	}
	if x.ID == 0 {
		r, e := s.DB.ExecContext(ctx, `INSERT INTO radio_stations(name,stream_url,genre,artwork,favorite,call_sign,frequency,city,region,country,market,station_type,format,description,website_url) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, x.Name, x.StreamURL, x.Genre, x.Artwork, boolInt(x.Favorite), x.CallSign, x.Frequency, x.City, x.Region, x.Country, x.Market, x.StationType, x.Format, x.Description, x.WebsiteURL)
		if e != nil {
			return 0, e
		}
		return r.LastInsertId()
	}
	_, e := s.DB.ExecContext(ctx, `UPDATE radio_stations SET name=?,stream_url=?,genre=?,artwork=?,favorite=?,call_sign=?,frequency=?,city=?,region=?,country=?,market=?,station_type=?,format=?,description=?,website_url=? WHERE id=?`, x.Name, x.StreamURL, x.Genre, x.Artwork, boolInt(x.Favorite), x.CallSign, x.Frequency, x.City, x.Region, x.Country, x.Market, x.StationType, x.Format, x.Description, x.WebsiteURL, x.ID)
	return x.ID, e
}
func (s *Store) DeleteRadio(ctx context.Context, id int64) error {
	_, e := s.DB.ExecContext(ctx, "DELETE FROM radio_stations WHERE id=?", id)
	return e
}
func (s *Store) Queue(ctx context.Context) ([]model.QueueItem, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT q.id,q.position,s.id,s.title,a.name,al.title,al.id,a.id,s.year,s.duration_ms,s.file_path,s.format,COALESCE(NULLIF(s.artwork,''),al.artwork),s.favorite FROM queue_items q JOIN songs s ON s.id=q.song_id JOIN artists a ON a.id=s.artist_id JOIN albums al ON al.id=s.album_id ORDER BY q.position`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]model.QueueItem, 0)
	for rows.Next() {
		var qi model.QueueItem
		var fav int
		if e := rows.Scan(&qi.ID, &qi.Position, &qi.Song.ID, &qi.Song.Title, &qi.Song.Artist, &qi.Song.Album, &qi.Song.AlbumID, &qi.Song.ArtistID, &qi.Song.Year, &qi.Song.DurationMS, &qi.Song.FilePath, &qi.Song.Format, &qi.Song.Artwork, &fav); e != nil {
			return nil, e
		}
		qi.Song.Favorite = fav != 0
		out = append(out, qi)
	}
	return out, rows.Err()
}
func (s *Store) RebuildShuffle(ctx context.Context) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, "DELETE FROM queue_shuffle"); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO queue_shuffle(position,queue_item_id) SELECT ROW_NUMBER() OVER(ORDER BY random())-1,id FROM queue_items`); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) QueueForPlayback(ctx context.Context, shuffle bool) ([]model.QueueItem, error) {
	if !shuffle {
		return s.Queue(ctx)
	}
	q, e := s.Queue(ctx)
	if e != nil {
		return nil, e
	}
	var count int
	if e = s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM queue_shuffle").Scan(&count); e != nil {
		return nil, e
	}
	if count != len(q) {
		if e = s.RebuildShuffle(ctx); e != nil {
			return nil, e
		}
	}
	rows, e := s.DB.QueryContext(ctx, "SELECT queue_item_id FROM queue_shuffle ORDER BY position")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	byID := make(map[int64]model.QueueItem, len(q))
	for _, x := range q {
		byID[x.ID] = x
	}
	out := make([]model.QueueItem, 0, len(q))
	for rows.Next() {
		var id int64
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		if x, ok := byID[id]; ok {
			out = append(out, x)
		}
	}
	return out, rows.Err()
}

func (s *Store) PlayerState(ctx context.Context) (model.PlayerState, error) {
	var p model.PlayerState
	var track, station sql.NullInt64
	var muted, shuffle int
	var updated string
	e := s.DB.QueryRowContext(ctx, `SELECT status,track_id,station_id,position_ms,volume,muted,shuffle,repeat_mode,queue_index,updated_at FROM player_state WHERE id=1`).Scan(&p.Status, &track, &station, &p.PositionMS, &p.Volume, &muted, &shuffle, &p.Repeat, &p.QueueIndex, &updated)
	if e != nil {
		return p, e
	}
	p.Muted = muted != 0
	p.Shuffle = shuffle != 0
	if track.Valid {
		p.TrackID = track.Int64
		if x, e := s.Song(ctx, p.TrackID); e == nil {
			p.CurrentSong = &x
			p.DurationMS = x.DurationMS
		}
	}
	if station.Valid {
		p.StationID = station.Int64
		if x, e := s.RadioByID(ctx, p.StationID); e == nil {
			p.CurrentRadio = &x
		}
	}
	var n int
	_ = s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM queue_items").Scan(&n)
	p.QueueLength = n
	return p, nil
}
func (s *Store) SavePlayerState(ctx context.Context, p model.PlayerState) error {
	_, e := s.DB.ExecContext(ctx, `UPDATE player_state SET status=?,track_id=NULLIF(?,0),station_id=NULLIF(?,0),position_ms=?,volume=?,muted=?,shuffle=?,repeat_mode=?,queue_index=?,updated_at=CURRENT_TIMESTAMP WHERE id=1`, p.Status, p.TrackID, p.StationID, p.PositionMS, p.Volume, boolInt(p.Muted), boolInt(p.Shuffle), p.Repeat, p.QueueIndex)
	return e
}

func (s *Store) Search(ctx context.Context, q string) (model.SearchResults, error) {
	like := "%" + strings.ToLower(strings.TrimSpace(q)) + "%"
	var out model.SearchResults
	if q == "" {
		return out, nil
	}
	rows, e := s.DB.QueryContext(ctx, songSelect+` WHERE lower(s.title) LIKE ? OR lower(a.name) LIKE ? OR lower(al.title) LIKE ? ORDER BY s.title LIMIT 20`, like, like, like)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		x, e := scanSong(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Songs = append(out.Songs, x)
	}
	rows.Close()
	albums, _ := s.Albums(ctx)
	for _, x := range albums {
		if strings.Contains(strings.ToLower(x.Title), strings.ToLower(q)) || strings.Contains(strings.ToLower(x.Artist), strings.ToLower(q)) {
			out.Albums = append(out.Albums, x)
		}
	}
	artists, _ := s.Artists(ctx)
	for _, x := range artists {
		if strings.Contains(strings.ToLower(x.Name), strings.ToLower(q)) {
			out.Artists = append(out.Artists, x)
		}
	}
	pls, _ := s.Playlists(ctx)
	for _, x := range pls {
		if strings.Contains(strings.ToLower(x.Name), strings.ToLower(q)) {
			out.Playlists = append(out.Playlists, x)
		}
	}
	rad, _ := s.Radio(ctx)
	for _, x := range rad {
		if strings.Contains(strings.ToLower(strings.Join([]string{x.Name, x.CallSign, x.Frequency, x.Genre, x.Format, x.StationType, x.City, x.Region, x.Market, x.Description}, " ")), strings.ToLower(q)) {
			out.Radio = append(out.Radio, x)
		}
	}
	return out, nil
}

func (s *Store) UpdateSong(ctx context.Context, id int64, title string, favorite *bool) error {
	if title != "" {
		if _, e := s.DB.ExecContext(ctx, "UPDATE songs SET title=?,updated_at=CURRENT_TIMESTAMP WHERE id=?", title, id); e != nil {
			return e
		}
	}
	if favorite != nil {
		_, e := s.DB.ExecContext(ctx, "UPDATE songs SET favorite=?,updated_at=CURRENT_TIMESTAMP WHERE id=?", boolInt(*favorite), id)
		return e
	}
	return nil
}
func (s *Store) LockSongMetadata(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, "UPDATE songs SET metadata_locked=1,metadata_source='manual',metadata_confidence=100,updated_at=CURRENT_TIMESTAMP WHERE id=?", id)
	return err
}
func (s *Store) UpdateSongMetadata(ctx context.Context, id int64, e model.Song) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO artists(name) VALUES(?) ON CONFLICT(name) DO NOTHING`, e.Artist); err != nil {
		return err
	}
	var artistID int64
	if err = tx.QueryRowContext(ctx, "SELECT id FROM artists WHERE name=?", e.Artist).Scan(&artistID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO albums(title,artist_id,year) VALUES(?,?,?) ON CONFLICT(title,artist_id,year) DO NOTHING`, e.Album, artistID, e.Year); err != nil {
		return err
	}
	var albumID int64
	if err = tx.QueryRowContext(ctx, "SELECT id FROM albums WHERE title=? AND artist_id=? AND year=?", e.Album, artistID, e.Year).Scan(&albumID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE songs SET title=?,artist_id=?,album_id=?,year=?,track_number=?,disc_number=?,genre=?,metadata_source=?,metadata_confidence=?,recording_mbid=?,release_group_mbid=?,release_mbid=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, e.Title, artistID, albumID, e.Year, e.TrackNumber, e.DiscNumber, e.Genre, e.MetadataSource, e.MetadataConfidence, e.RecordingMBID, e.ReleaseGroupMBID, e.ReleaseMBID, id); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) DeleteSong(ctx context.Context, id int64) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var albumID, artistID int64
	if e = tx.QueryRowContext(ctx, "SELECT album_id,artist_id FROM songs WHERE id=?", id).Scan(&albumID, &artistID); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE import_jobs SET song_id=NULL WHERE song_id=?", id); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE import_jobs SET existing_song_id=NULL WHERE existing_song_id=?", id); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE player_state SET track_id=NULL,status='stopped',position_ms=0 WHERE track_id=?", id); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM songs WHERE id=?", id); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM albums WHERE id=? AND NOT EXISTS(SELECT 1 FROM songs WHERE album_id=?)", albumID, albumID); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM artists WHERE id=? AND NOT EXISTS(SELECT 1 FROM songs WHERE artist_id=?)", artistID, artistID); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) DeleteSongs(ctx context.Context, ids []int64) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	albums, artists := map[int64]bool{}, map[int64]bool{}
	for _, id := range ids {
		var albumID, artistID int64
		if e = tx.QueryRowContext(ctx, "SELECT album_id,artist_id FROM songs WHERE id=?", id).Scan(&albumID, &artistID); e != nil {
			return e
		}
		albums[albumID], artists[artistID] = true, true
		if _, e = tx.ExecContext(ctx, "UPDATE import_jobs SET song_id=NULL WHERE song_id=?", id); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "UPDATE import_jobs SET existing_song_id=NULL WHERE existing_song_id=?", id); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "UPDATE player_state SET track_id=NULL,status='stopped',position_ms=0 WHERE track_id=?", id); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "DELETE FROM songs WHERE id=?", id); e != nil {
			return e
		}
	}
	for id := range albums {
		if _, e = tx.ExecContext(ctx, "DELETE FROM albums WHERE id=? AND NOT EXISTS(SELECT 1 FROM songs WHERE album_id=?)", id, id); e != nil {
			return e
		}
	}
	for id := range artists {
		if _, e = tx.ExecContext(ctx, "DELETE FROM artists WHERE id=? AND NOT EXISTS(SELECT 1 FROM songs WHERE artist_id=?)", id, id); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func (s *Store) UpdateSongArtwork(ctx context.Context, id int64, path string) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `UPDATE albums SET artwork=? WHERE id=(SELECT album_id FROM songs WHERE id=?)`, path, id); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE songs SET artwork=?,updated_at=CURRENT_TIMESTAMP WHERE album_id=(SELECT album_id FROM songs WHERE id=?)`, path, id); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) MoveSongPathPrefix(ctx context.Context, oldPath, newPath string) error {
	_, e := s.DB.ExecContext(ctx, `UPDATE songs SET file_path=? || substr(file_path,length(?)+1),updated_at=CURRENT_TIMESTAMP WHERE file_path LIKE ?`, newPath, oldPath, oldPath+string(filepath.Separator)+"%")
	return e
}
func (s *Store) UpdateSongPaths(ctx context.Context, paths map[int64]string) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for id, path := range paths {
		result, err := tx.ExecContext(ctx, "UPDATE songs SET file_path=?,updated_at=CURRENT_TIMESTAMP WHERE id=?", path, id)
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return errors.New("song not found")
		}
	}
	return tx.Commit()
}
func (s *Store) CreatePlaylist(ctx context.Context, name string) (int64, error) {
	palettes := []string{"aurora", "cobalt", "sunset", "orchid", "forest", "ember", "lagoon", "berry"}
	var choice [1]byte
	_, _ = rand.Read(choice[:])
	r, e := s.DB.ExecContext(ctx, "INSERT INTO playlists(name,artwork) VALUES(?,?)", name, "playlist-gradient:"+palettes[int(choice[0])%len(palettes)])
	if e != nil {
		return 0, e
	}
	return r.LastInsertId()
}
func (s *Store) UpdatePlaylistArtwork(ctx context.Context, id int64, artwork string) error {
	result, err := s.DB.ExecContext(ctx, `UPDATE playlists SET artwork=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, artwork, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return sql.ErrNoRows
	}
	return err
}
func (s *Store) DeletePlaylist(ctx context.Context, id int64) error {
	_, e := s.DB.ExecContext(ctx, "DELETE FROM playlists WHERE id=?", id)
	return e
}
func (s *Store) AddPlaylistSong(ctx context.Context, playlistID, songID int64) error {
	var pos int
	_ = s.DB.QueryRowContext(ctx, "SELECT COALESCE(MAX(position),-1)+1 FROM playlist_items WHERE playlist_id=?", playlistID).Scan(&pos)
	_, e := s.DB.ExecContext(ctx, "INSERT INTO playlist_items(playlist_id,song_id,position) VALUES(?,?,?)", playlistID, songID, pos)
	return e
}
func (s *Store) AddPlaylistSongs(ctx context.Context, playlistID int64, songIDs []int64) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var pos int
	if e = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(position),-1)+1 FROM playlist_items WHERE playlist_id=?", playlistID).Scan(&pos); e != nil {
		return e
	}
	seen := map[int64]bool{}
	for _, songID := range songIDs {
		if seen[songID] {
			continue
		}
		seen[songID] = true
		result, err := tx.ExecContext(ctx, `INSERT INTO playlist_items(playlist_id,song_id,position) SELECT ?,?,? WHERE EXISTS(SELECT 1 FROM songs WHERE id=?) AND NOT EXISTS(SELECT 1 FROM playlist_items WHERE playlist_id=? AND song_id=?)`, playlistID, songID, pos, songID, playlistID, songID)
		if err != nil {
			return err
		}
		if n, _ := result.RowsAffected(); n == 1 {
			pos++
		}
	}
	return tx.Commit()
}
func (s *Store) RemovePlaylistSong(ctx context.Context, playlistID, songID int64) error {
	_, e := s.DB.ExecContext(ctx, "DELETE FROM playlist_items WHERE playlist_id=? AND song_id=?", playlistID, songID)
	return e
}
func (s *Store) ReorderPlaylist(ctx context.Context, playlistID int64, songIDs []int64) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var count int
	if e = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM playlist_items WHERE playlist_id=?", playlistID).Scan(&count); e != nil {
		return e
	}
	if count != len(songIDs) {
		return errors.New("song list must include every playlist item")
	}
	if _, e = tx.ExecContext(ctx, "UPDATE playlist_items SET position=position+1000000 WHERE playlist_id=?", playlistID); e != nil {
		return e
	}
	seen := map[int64]bool{}
	for position, songID := range songIDs {
		if seen[songID] {
			return errors.New("duplicate song")
		}
		seen[songID] = true
		result, err := tx.ExecContext(ctx, "UPDATE playlist_items SET position=? WHERE playlist_id=? AND song_id=?", position, playlistID, songID)
		if err != nil {
			return err
		}
		affected, _ := result.RowsAffected()
		if affected != 1 {
			return errors.New("unknown playlist song")
		}
	}
	return tx.Commit()
}
func (s *Store) AddQueue(ctx context.Context, songID int64) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var pos int
	if e = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(position),-1)+1 FROM queue_items").Scan(&pos); e != nil {
		return e
	}
	r, e := tx.ExecContext(ctx, "INSERT INTO queue_items(song_id,position) SELECT ?,? WHERE EXISTS(SELECT 1 FROM songs WHERE id=?)", songID, pos, songID)
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return errors.New("song not found")
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	var shuffled int
	_ = s.DB.QueryRowContext(ctx, "SELECT shuffle FROM player_state WHERE id=1").Scan(&shuffled)
	if shuffled != 0 {
		_, e = s.DB.ExecContext(ctx, `INSERT INTO queue_shuffle(position,queue_item_id) SELECT COALESCE(MAX(position),-1)+1,(SELECT id FROM queue_items ORDER BY position DESC LIMIT 1) FROM queue_shuffle`)
	}
	return e
}
func (s *Store) ReplaceQueue(ctx context.Context, songIDs []int64) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, "DELETE FROM queue_items"); e != nil {
		return e
	}
	seen := map[int64]bool{}
	for position, songID := range songIDs {
		if seen[songID] {
			return errors.New("duplicate queue song")
		}
		seen[songID] = true
		result, err := tx.ExecContext(ctx, "INSERT INTO queue_items(song_id,position) SELECT ?,? WHERE EXISTS(SELECT 1 FROM songs WHERE id=?)", songID, position, songID)
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return errors.New("song not found")
		}
	}
	return tx.Commit()
}
func (s *Store) ClearQueue(ctx context.Context) error {
	_, e := s.DB.ExecContext(ctx, "DELETE FROM queue_items")
	return e
}
func (s *Store) RemoveQueue(ctx context.Context, id int64) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, "DELETE FROM queue_items WHERE id=?", id); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `WITH ranked AS (SELECT id,ROW_NUMBER() OVER(ORDER BY position)-1 p FROM queue_items) UPDATE queue_items SET position=(SELECT p FROM ranked WHERE ranked.id=queue_items.id)`); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `WITH ranked AS (SELECT queue_item_id,ROW_NUMBER() OVER(ORDER BY position)-1 p FROM queue_shuffle) UPDATE queue_shuffle SET position=(SELECT p FROM ranked WHERE ranked.queue_item_id=queue_shuffle.queue_item_id)`); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) ReorderQueue(ctx context.Context, ids []int64) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var count int
	if e = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM queue_items").Scan(&count); e != nil {
		return e
	}
	if count != len(ids) {
		return errors.New("queue item list must include every item")
	}
	if _, e = tx.ExecContext(ctx, "UPDATE queue_items SET position=position+1000000"); e != nil {
		return e
	}
	seen := map[int64]bool{}
	for i, id := range ids {
		if seen[id] {
			return errors.New("duplicate queue item")
		}
		seen[id] = true
		r, e := tx.ExecContext(ctx, "UPDATE queue_items SET position=? WHERE id=?", i, id)
		if e != nil {
			return e
		}
		n, _ := r.RowsAffected()
		if n != 1 {
			return errors.New("unknown queue item")
		}
	}
	return tx.Commit()
}
func (s *Store) RecordPlay(ctx context.Context, id int64) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, "INSERT INTO play_history(song_id) VALUES(?)", id); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE songs SET play_count=play_count+1,last_played_at=CURRENT_TIMESTAMP WHERE id=?", id); e != nil {
		return e
	}
	return tx.Commit()
}

func (s *Store) CreateImportJob(ctx context.Context, filename, source string) (int64, error) {
	r, err := s.DB.ExecContext(ctx, `INSERT INTO import_jobs(filename,source_path,status) VALUES(?,?,'queued')`, filename, source)
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}

func (s *Store) ClaimImportJob(ctx context.Context) (int64, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `UPDATE import_jobs
		SET status='processing',message='Starting import',updated_at=CURRENT_TIMESTAMP
		WHERE id=(SELECT id FROM import_jobs WHERE status='queued' ORDER BY id LIMIT 1)
		RETURNING id`).Scan(&id)
	return id, err
}

func (s *Store) RequeueInterruptedImports(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE import_jobs
		SET status='queued',message='Resuming after restart',updated_at=CURRENT_TIMESTAMP
		WHERE status='processing'`)
	return err
}

func (s *Store) SetImportTarget(ctx context.Context, id int64, folder string) error {
	_, err := s.DB.ExecContext(ctx, "UPDATE import_jobs SET target_folder=? WHERE id=?", folder, id)
	return err
}

func (s *Store) ImportJob(ctx context.Context, id int64) (model.ImportJob, error) {
	var x model.ImportJob
	err := s.DB.QueryRowContext(ctx, `SELECT id,filename,source_path,target_folder,status,message,COALESCE(song_id,0),COALESCE(existing_song_id,0),created_at,updated_at FROM import_jobs WHERE id=?`, id).
		Scan(&x.ID, &x.Filename, &x.SourcePath, &x.TargetFolder, &x.Status, &x.Message, &x.SongID, &x.ExistingSongID, &x.CreatedAt, &x.UpdatedAt)
	return x, err
}

func (s *Store) ImportJobs(ctx context.Context) ([]model.ImportJob, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,filename,source_path,target_folder,status,message,COALESCE(song_id,0),COALESCE(existing_song_id,0),created_at,updated_at FROM import_jobs ORDER BY id DESC LIMIT 5000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.ImportJob, 0)
	for rows.Next() {
		var x model.ImportJob
		if err := rows.Scan(&x.ID, &x.Filename, &x.SourcePath, &x.TargetFolder, &x.Status, &x.Message, &x.SongID, &x.ExistingSongID, &x.CreatedAt, &x.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) UpdateImportJob(ctx context.Context, id int64, status, message string, songID, existingID int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE import_jobs SET status=?,message=?,song_id=NULLIF(?,0),existing_song_id=NULLIF(?,0),updated_at=CURRENT_TIMESTAMP WHERE id=?`, status, message, songID, existingID, id)
	return err
}

func (s *Store) SongByHash(ctx context.Context, hash string) (model.Song, error) {
	return scanSong(s.DB.QueryRowContext(ctx, songSelect+" WHERE s.sha256=?", hash))
}

type ImportedSong struct {
	Title, Artist, Album, FilePath, Format, Artwork, SHA256, OriginalFilename, Codec, Genre string
	Year, TrackNumber, DiscNumber, SampleRate, Channels                                     int
	DurationMS, Bitrate, FileSize                                                           int64
}

func (s *Store) InsertImportedSong(ctx context.Context, x ImportedSong) (int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	artist := strings.TrimSpace(x.Artist)
	if artist == "" {
		artist = "Unknown Artist"
	}
	album := strings.TrimSpace(x.Album)
	if album == "" {
		album = "Unknown Album"
	}
	title := strings.TrimSpace(x.Title)
	if title == "" {
		title = strings.TrimSuffix(x.OriginalFilename, filepath.Ext(x.OriginalFilename))
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO artists(name) VALUES(?) ON CONFLICT(name) DO NOTHING`, artist); err != nil {
		return 0, err
	}
	var artistID int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM artists WHERE name=?`, artist).Scan(&artistID); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO albums(title,artist_id,year) VALUES(?,?,?) ON CONFLICT(title,artist_id,year) DO NOTHING`, album, artistID, x.Year); err != nil {
		return 0, err
	}
	var albumID int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM albums WHERE title=? AND artist_id=? AND year=?`, album, artistID, x.Year).Scan(&albumID); err != nil {
		return 0, err
	}
	r, err := tx.ExecContext(ctx, `INSERT INTO songs(title,artist_id,album_id,year,duration_ms,file_path,format,artwork,sha256,track_number,disc_number,codec,bitrate,sample_rate,channels,file_size,original_filename,genre,imported_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP)`, title, artistID, albumID, x.Year, x.DurationMS, x.FilePath, x.Format, x.Artwork, x.SHA256, x.TrackNumber, x.DiscNumber, x.Codec, x.Bitrate, x.SampleRate, x.Channels, x.FileSize, x.OriginalFilename, x.Genre)
	if err != nil {
		return 0, err
	}
	id, err := r.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (s *Store) LibraryStats(ctx context.Context) (model.LibraryStats, error) {
	var x model.LibraryStats
	err := s.DB.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM songs),(SELECT COUNT(*) FROM albums),(SELECT COUNT(*) FROM artists),(SELECT COUNT(*) FROM radio_stations WHERE enabled=1),(SELECT COUNT(*) FROM songs WHERE artwork=''),(SELECT COUNT(*) FROM songs WHERE title='' OR duration_ms=0),(SELECT COUNT(*) FROM (SELECT sha256 FROM songs WHERE sha256<>'' GROUP BY sha256 HAVING COUNT(*)>1))`).Scan(&x.Songs, &x.Albums, &x.Artists, &x.Stations, &x.MissingArtwork, &x.MetadataIssues, &x.DuplicateFiles)
	return x, err
}

func (s *Store) PlaylistSongs(ctx context.Context, id int64) ([]model.Song, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT s.id,s.title,a.name,al.title,al.id,a.id,s.year,s.duration_ms,s.file_path,s.format,COALESCE(NULLIF(s.artwork,''),al.artwork),s.favorite,s.track_number,s.disc_number,s.codec,s.bitrate,s.sample_rate,s.channels,s.file_size,s.sha256,s.original_filename,s.imported_at,s.genre,s.metadata_source,s.metadata_confidence,s.recording_mbid,s.release_group_mbid,s.release_mbid,s.metadata_locked
		FROM playlist_items pi JOIN songs s ON s.id=pi.song_id JOIN artists a ON a.id=s.artist_id JOIN albums al ON al.id=s.album_id
		WHERE pi.playlist_id=? ORDER BY pi.position`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.Song, 0)
	for rows.Next() {
		x, e := scanSong(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
