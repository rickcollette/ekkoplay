package model

import "time"

type Song struct {
	ID                 int64  `json:"id"`
	Title              string `json:"title"`
	Artist             string `json:"artist"`
	Album              string `json:"album"`
	AlbumID            int64  `json:"album_id"`
	ArtistID           int64  `json:"artist_id"`
	Year               int    `json:"year"`
	DurationMS         int64  `json:"duration_ms"`
	FilePath           string `json:"file_path,omitempty"`
	Format             string `json:"format"`
	Artwork            string `json:"artwork,omitempty"`
	Favorite           bool   `json:"favorite"`
	TrackNumber        int    `json:"track_number"`
	DiscNumber         int    `json:"disc_number"`
	Codec              string `json:"codec"`
	Bitrate            int64  `json:"bitrate"`
	SampleRate         int    `json:"sample_rate"`
	Channels           int    `json:"channels"`
	FileSize           int64  `json:"file_size"`
	SHA256             string `json:"sha256,omitempty"`
	OriginalFilename   string `json:"original_filename"`
	ImportedAt         string `json:"imported_at"`
	Genre              string `json:"genre"`
	MetadataSource     string `json:"metadata_source,omitempty"`
	MetadataConfidence int    `json:"metadata_confidence,omitempty"`
	RecordingMBID      string `json:"recording_mbid,omitempty"`
	ReleaseGroupMBID   string `json:"release_group_mbid,omitempty"`
	ReleaseMBID        string `json:"release_mbid,omitempty"`
	MetadataLocked     bool   `json:"metadata_locked,omitempty"`
}

type ImportJob struct {
	ID             int64  `json:"id"`
	Filename       string `json:"filename"`
	SourcePath     string `json:"-"`
	TargetFolder   string `json:"target_folder,omitempty"`
	Status         string `json:"status"`
	Message        string `json:"message"`
	SongID         int64  `json:"song_id,omitempty"`
	ExistingSongID int64  `json:"existing_song_id,omitempty"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type TorrentJob struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	TargetFolder    string  `json:"target_folder"`
	TorrentHash     string  `json:"torrent_hash,omitempty"`
	Status          string  `json:"status"`
	Percent         float64 `json:"percent"`
	DownloadRate    int64   `json:"download_rate"`
	UploadRate      int64   `json:"upload_rate"`
	DownloadedBytes int64   `json:"downloaded_bytes"`
	UploadedBytes   int64   `json:"uploaded_bytes"`
	TotalBytes      int64   `json:"total_bytes"`
	Peers           int     `json:"peers"`
	ImportedCount   int     `json:"imported_count"`
	TotalAudio      int     `json:"total_audio"`
	Error           string  `json:"error,omitempty"`
	CompletedAt     string  `json:"completed_at,omitempty"`
	SeedUntil       string  `json:"seed_until,omitempty"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type LibraryStats struct {
	Songs          int64 `json:"songs"`
	Albums         int64 `json:"albums"`
	Artists        int64 `json:"artists"`
	Stations       int64 `json:"stations"`
	MissingArtwork int64 `json:"missing_artwork"`
	MetadataIssues int64 `json:"metadata_issues"`
	DuplicateFiles int64 `json:"duplicate_files"`
}
type Page[T any] struct {
	Items    []T `json:"items"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}

type Album struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	ArtistID  int64  `json:"artist_id"`
	Year      int    `json:"year"`
	Artwork   string `json:"artwork,omitempty"`
	SongCount int    `json:"song_count"`
}

type Artist struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	AlbumCount int    `json:"album_count"`
	SongCount  int    `json:"song_count"`
	Artwork    string `json:"artwork,omitempty"`
}

type Playlist struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	SongCount int    `json:"song_count"`
	Artwork   string `json:"artwork,omitempty"`
	UpdatedAt string `json:"updated_at"`
}

type RadioStation struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	StreamURL   string `json:"stream_url"`
	Genre       string `json:"genre"`
	Artwork     string `json:"artwork,omitempty"`
	Favorite    bool   `json:"favorite"`
	CallSign    string `json:"call_sign"`
	Frequency   string `json:"frequency"`
	City        string `json:"city"`
	Region      string `json:"region"`
	Country     string `json:"country"`
	Market      string `json:"market"`
	StationType string `json:"station_type"`
	Format      string `json:"format"`
	Description string `json:"description"`
	WebsiteURL  string `json:"website_url"`
	Enabled     bool   `json:"enabled"`
}

type QueueItem struct {
	ID       int64 `json:"id"`
	Position int   `json:"position"`
	Song     Song  `json:"song"`
}

type PlayerState struct {
	Status       string        `json:"status"`
	TrackID      int64         `json:"track_id,omitempty"`
	StationID    int64         `json:"station_id,omitempty"`
	PositionMS   int64         `json:"position_ms"`
	DurationMS   int64         `json:"duration_ms"`
	Volume       int           `json:"volume"`
	Muted        bool          `json:"muted"`
	Shuffle      bool          `json:"shuffle"`
	Repeat       string        `json:"repeat"`
	QueueIndex   int           `json:"queue_index"`
	QueueLength  int           `json:"queue_length"`
	CurrentSong  *Song         `json:"current_song,omitempty"`
	CurrentRadio *RadioStation `json:"current_radio,omitempty"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

type SearchResults struct {
	Songs     []Song         `json:"songs"`
	Albums    []Album        `json:"albums"`
	Artists   []Artist       `json:"artists"`
	Playlists []Playlist     `json:"playlists"`
	Radio     []RadioStation `json:"radio"`
}

type HomeResponse struct {
	Player         PlayerState    `json:"player"`
	RecentlyPlayed []Song         `json:"recently_played"`
	RecentlyAdded  []Song         `json:"recently_added"`
	Favorites      []Song         `json:"favorites"`
	Playlists      []Playlist     `json:"playlists"`
	Radio          []RadioStation `json:"radio"`
}
