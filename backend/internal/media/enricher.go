package media

import (
	"context"
	"database/sql"
	"ekkoplayer/internal/config"
	"ekkoplayer/internal/db"
	"ekkoplayer/internal/model"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
)

type Enricher struct {
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	store     *db.Store
	cfg       config.Config
	broadcast BroadcastFunc
	client    *http.Client
	running   atomic.Bool
	mbMu      sync.Mutex
	lastMB    time.Time
}
type EnrichmentStats struct {
	Pending    int  `json:"pending"`
	Processing int  `json:"processing"`
	Complete   int  `json:"complete"`
	Retry      int  `json:"retry"`
	Unmatched  int  `json:"unmatched"`
	Running    bool `json:"running"`
}
type artistCredit struct {
	Name       string `json:"name"`
	JoinPhrase string `json:"joinphrase"`
}
type mbReleaseGroup struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	FirstReleaseDate string   `json:"first-release-date"`
	PrimaryType      string   `json:"primary-type"`
	SecondaryTypes   []string `json:"secondary-types"`
}
type mbRecordingRelease struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	Date         string         `json:"date"`
	Status       string         `json:"status"`
	ReleaseGroup mbReleaseGroup `json:"release-group"`
}
type mbRecording struct {
	ID           string               `json:"id"`
	Title        string               `json:"title"`
	Score        int                  `json:"score"`
	Length       int64                `json:"length"`
	ArtistCredit []artistCredit       `json:"artist-credit"`
	Releases     []mbRecordingRelease `json:"releases"`
}
type mbResponse struct {
	Recordings []mbRecording `json:"recordings"`
}
type mbGenre struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}
type mbGroupLookup struct {
	Genres []mbGenre `json:"genres"`
}
type mbRelease struct {
	Media []struct {
		Tracks []struct {
			Position  int `json:"position"`
			Recording struct {
				ID string `json:"id"`
			} `json:"recording"`
		} `json:"tracks"`
	} `json:"media"`
}
type acoustIDResponse struct {
	Results []struct {
		Score      float64 `json:"score"`
		Recordings []struct {
			ID string `json:"id"`
		} `json:"recordings"`
	} `json:"results"`
}

var bracketNoise = regexp.MustCompile(`(?i)\s*[\[(][^\])]*(official|music\s*video|audio|lyrics?|visuali[sz]er|hd|4k|hq)[^\])]*[\])]\s*`)
var tailNoise = regexp.MustCompile(`(?i)\s*[-|:]?\s*(official\s*)?(music\s*)?(video|audio|lyrics?|visuali[sz]er)(\s*(hd|hq|4k))?\s*$`)
var duplicateSuffix = regexp.MustCompile(`\s*\(\d+\)\s*$`)
var leadingTrack = regexp.MustCompile(`^\s*\d{1,3}\s*[-._]\s*`)
var whitespace = regexp.MustCompile(`\s+`)

func NewEnricher(cfg config.Config, store *db.Store, broadcast BroadcastFunc) *Enricher {
	ctx, cancel := context.WithCancel(context.Background())
	e := &Enricher{cancel: cancel, store: store, cfg: cfg, broadcast: broadcast, client: &http.Client{Timeout: 25 * time.Second}}
	e.wg.Add(1)
	go e.loop(ctx)
	return e
}
func (e *Enricher) Close() { e.cancel(); e.wg.Wait() }
func (e *Enricher) loop(ctx context.Context) {
	defer e.wg.Done()
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			e.RunBatch(ctx, 25)
			timer.Reset(10 * time.Minute)
		}
	}
}
func (e *Enricher) RunBatch(ctx context.Context, limit int) {
	if !e.running.CompareAndSwap(false, true) {
		return
	}
	defer e.running.Store(false)
	rows, err := e.store.DB.QueryContext(ctx, `SELECT s.id FROM songs s LEFT JOIN song_enrichment x ON x.song_id=s.id WHERE s.metadata_locked=0 AND (x.song_id IS NULL OR (x.status='retry' AND (x.next_attempt_at IS NULL OR x.next_attempt_at<=CURRENT_TIMESTAMP))) ORDER BY COALESCE(x.attempts,0),s.id LIMIT ?`, limit)
	if err != nil {
		return
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		e.process(ctx, id)
	}
}
func (e *Enricher) Stats(ctx context.Context) (EnrichmentStats, error) {
	var s EnrichmentStats
	err := e.store.DB.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM songs s LEFT JOIN song_enrichment x ON x.song_id=s.id WHERE s.metadata_locked=0 AND (x.song_id IS NULL OR x.status='pending')),
		COUNT(*) FILTER(WHERE status='processing'),COUNT(*) FILTER(WHERE status='complete'),COUNT(*) FILTER(WHERE status='retry'),COUNT(*) FILTER(WHERE status='unmatched')
		FROM song_enrichment`).Scan(&s.Pending, &s.Processing, &s.Complete, &s.Retry, &s.Unmatched)
	s.Running = e.running.Load()
	return s, err
}
func (e *Enricher) Retry(ctx context.Context) error {
	_, err := e.store.DB.ExecContext(ctx, `UPDATE song_enrichment SET status='retry',attempts=0,next_attempt_at=CURRENT_TIMESTAMP,message='Manual retry',updated_at=CURRENT_TIMESTAMP WHERE status IN ('unmatched','retry')`)
	return err
}

func cleanDisplay(v string) string {
	ext := strings.ToLower(filepath.Ext(v))
	switch ext {
	case ".mp3", ".m4a", ".mp4", ".flac", ".wav", ".ogg", ".opus", ".aac", ".wma", ".alac", ".aiff":
		v = strings.TrimSuffix(v, filepath.Ext(v))
	}
	v = duplicateSuffix.ReplaceAllString(v, "")
	v = bracketNoise.ReplaceAllString(v, " ")
	v = tailNoise.ReplaceAllString(v, "")
	v = leadingTrack.ReplaceAllString(v, "")
	return strings.TrimSpace(whitespace.ReplaceAllString(v, " "))
}
func parseName(s model.Song) (string, string) {
	stem := cleanDisplay(s.OriginalFilename)
	for _, sep := range []string{" - ", " – ", " — ", " | "} {
		if p := strings.SplitN(stem, sep, 2); len(p) == 2 {
			left, right := strings.TrimSpace(p[0]), strings.TrimSpace(p[1])
			if strings.Contains(strings.ToLower(left), " lyrics") && !strings.Contains(strings.ToLower(right), " lyrics") {
				return cleanDisplay(right), cleanDisplay(regexp.MustCompile(`(?i)\s*lyrics?`).ReplaceAllString(left, ""))
			}
			return cleanDisplay(left), cleanDisplay(right)
		}
	}
	artist := s.Artist
	if useful(artist) == "" {
		artist = ""
	}
	title := cleanDisplay(s.Title)
	if title == "" {
		title = stem
	}
	return artist, title
}
func normalized(v string) string {
	var b strings.Builder
	lastSpace := true
	for _, r := range strings.ToLower(cleanDisplay(v)) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
			lastSpace = false
		} else if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}
func useful(v string) string {
	n := normalized(v)
	if n == "" || n == "unknown" || strings.HasPrefix(n, "unknown ") || n == "various artists" {
		return ""
	}
	return n
}
func tokens(v string) map[string]bool {
	out := map[string]bool{}
	for _, x := range strings.Fields(normalized(v)) {
		if len([]rune(x)) > 1 {
			out[x] = true
		}
	}
	return out
}
func similarity(a, b string) float64 {
	aa, bb := tokens(a), tokens(b)
	if len(aa) == 0 || len(bb) == 0 {
		return 0
	}
	n := 0
	for x := range aa {
		if bb[x] {
			n++
		}
	}
	return float64(2*n) / float64(len(aa)+len(bb))
}
func creditName(in []artistCredit) string {
	var b strings.Builder
	for _, a := range in {
		b.WriteString(a.Name)
		b.WriteString(a.JoinPhrase)
	}
	return strings.TrimSpace(b.String())
}
func candidateScore(song model.Song, artist, title string, r mbRecording) (int, int) {
	artistName := creditName(r.ArtistCredit)
	ts := similarity(title, r.Title)
	combined := similarity(strings.TrimSpace(artist+" "+title), artistName+" "+r.Title)
	signals, score := 0, 0
	if ts >= .82 {
		signals++
		score += int(ts * 45)
	} else if artist == "" && combined >= .72 {
		signals++
		score += int(combined * 40)
	}
	if useful(artist) != "" && similarity(artist, artistName) >= .78 {
		signals++
		score += 35
	}
	if song.DurationMS > 0 && r.Length > 0 {
		d := abs(song.DurationMS - r.Length)
		if d <= 4000 {
			signals++
			score += 35
		} else if d <= 10000 {
			signals++
			score += 25
		}
	}
	if album := useful(song.Album); album != "" {
		for _, release := range r.Releases {
			if similarity(album, release.ReleaseGroup.Title) >= .9 || similarity(album, release.Title) >= .9 {
				signals++
				score += 20
				break
			}
		}
	}
	return signals, score + r.Score/10
}
func bestRecording(song model.Song, artist, title string, items []mbRecording) (mbRecording, int, bool) {
	var best mbRecording
	bestScore := -1
	for _, r := range items {
		signals, score := candidateScore(song, artist, title, r)
		if signals < 2 || len(r.ArtistCredit) == 0 || len(r.Releases) == 0 || r.Score < 60 {
			continue
		}
		if score > bestScore {
			best, bestScore = r, score
		}
	}
	confidence := bestScore
	if confidence > 99 {
		confidence = 99
	}
	return best, confidence, bestScore >= 0
}
func releasePenalty(g mbReleaseGroup) int {
	p := 0
	for _, t := range g.SecondaryTypes {
		switch strings.ToLower(t) {
		case "compilation":
			p += 100
		case "live", "remix", "dj-mix", "mixtape/street", "bootleg":
			p += 70
		case "soundtrack":
			p += 20
		}
	}
	return p
}
func chooseRelease(song model.Song, releases []mbRecordingRelease) mbRecordingRelease {
	best := releases[0]
	bestScore := -10000
	for _, r := range releases {
		score := 0
		if strings.EqualFold(r.Status, "Official") {
			score += 25
		}
		if r.ReleaseGroup.ID != "" {
			score += 20
		}
		switch strings.ToLower(r.ReleaseGroup.PrimaryType) {
		case "album":
			score += 35
		case "single":
			score += 18
		case "ep":
			score += 12
		}
		score -= releasePenalty(r.ReleaseGroup)
		if album := useful(song.Album); album != "" && similarity(album, r.ReleaseGroup.Title) >= .9 {
			score += 100
		}
		year := yearOf(r.ReleaseGroup.FirstReleaseDate)
		if year == 0 {
			year = yearOf(r.Date)
		}
		if year > 0 {
			score += max(0, 30-(year-1900)/5)
		}
		if score > bestScore {
			best, bestScore = r, score
		}
	}
	return best
}
func yearOf(v string) int {
	if len(v) < 4 {
		return 0
	}
	n, _ := strconv.Atoi(v[:4])
	return n
}

func (e *Enricher) mbGet(ctx context.Context, path string, out any) error {
	e.mbMu.Lock()
	wait := 1100*time.Millisecond - time.Since(e.lastMB)
	if wait > 0 {
		select {
		case <-ctx.Done():
			e.mbMu.Unlock()
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	e.lastMB = time.Now()
	e.mbMu.Unlock()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://musicbrainz.org/ws/2/"+path, nil)
	req.Header.Set("User-Agent", "ekkoPlayer/0.3 (self-hosted music appliance)")
	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("MusicBrainz: %s", resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out)
}
func (e *Enricher) fingerprint(ctx context.Context, s model.Song) (string, float64) {
	if e.cfg.AcoustIDKey == "" || e.cfg.FPCalcBinary == "" {
		return "", 0
	}
	raw, err := exec.CommandContext(ctx, e.cfg.FPCalcBinary, "-json", "-length", "120", s.FilePath).Output()
	if err != nil {
		return "", 0
	}
	var fp struct {
		Duration    float64 `json:"duration"`
		Fingerprint string  `json:"fingerprint"`
	}
	if json.Unmarshal(raw, &fp) != nil || fp.Fingerprint == "" {
		return "", 0
	}
	v := url.Values{"client": {e.cfg.AcoustIDKey}, "duration": {strconv.Itoa(int(fp.Duration + .5))}, "fingerprint": {fp.Fingerprint}, "meta": {"recordingids"}}
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.acoustid.org/v2/lookup?"+v.Encode(), nil)
	resp, err := e.client.Do(req)
	if err != nil {
		return "", 0
	}
	defer resp.Body.Close()
	var data acoustIDResponse
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&data) != nil {
		return "", 0
	}
	for _, r := range data.Results {
		if r.Score >= .8 && len(r.Recordings) > 0 {
			return r.Recordings[0].ID, r.Score
		}
	}
	return "", 0
}
func (e *Enricher) identify(ctx context.Context, s model.Song, artist, title string) (mbRecording, int, string, error) {
	if mbid, score := e.fingerprint(ctx, s); mbid != "" {
		var r mbRecording
		if err := e.mbGet(ctx, "recording/"+url.PathEscape(mbid)+"?inc=artist-credits+releases+release-groups&fmt=json", &r); err == nil && len(r.Releases) > 0 {
			return r, int(score * 100), "acoustid", nil
		}
	}
	q := `recording:"` + strings.ReplaceAll(title, `"`, ``) + `"`
	if artist != "" {
		q += ` AND artist:"` + strings.ReplaceAll(artist, `"`, ``) + `"`
	}
	var data mbResponse
	err := e.mbGet(ctx, "recording?fmt=json&limit=15&query="+url.QueryEscape(q), &data)
	if err != nil {
		return mbRecording{}, 0, "", err
	}
	r, confidence, ok := bestRecording(s, artist, title, data.Recordings)
	if !ok && artist == "" {
		err = e.mbGet(ctx, "recording?fmt=json&limit=25&query="+url.QueryEscape(cleanDisplay(title)), &data)
		if err == nil {
			r, confidence, ok = bestRecording(s, artist, title, data.Recordings)
		}
	}
	if !ok {
		return mbRecording{}, 0, "", sql.ErrNoRows
	}
	return r, confidence, "text+duration", nil
}
func (e *Enricher) process(ctx context.Context, id int64) {
	song, err := e.store.Song(ctx, id)
	if err != nil || song.MetadataLocked {
		return
	}
	_, _ = e.store.DB.ExecContext(ctx, `INSERT INTO song_enrichment(song_id,status,attempts) VALUES(?,'processing',1) ON CONFLICT(song_id) DO UPDATE SET status='processing',attempts=attempts+1,updated_at=CURRENT_TIMESTAMP`, id)
	var attempts int
	_ = e.store.DB.QueryRowContext(ctx, `SELECT attempts FROM song_enrichment WHERE song_id=?`, id).Scan(&attempts)
	artist, title := parseName(song)
	if title == "" {
		e.fail(ctx, id, "No usable title")
		return
	}
	recording, confidence, method, err := e.identify(ctx, song, artist, title)
	if errors.Is(err, sql.ErrNoRows) {
		e.unmatched(ctx, id, "No candidate matched independent facts")
		return
	}
	if err != nil {
		e.fail(ctx, id, err.Error())
		return
	}
	release := chooseRelease(song, recording.Releases)
	group := release.ReleaseGroup
	year := yearOf(group.FirstReleaseDate)
	if year == 0 {
		year = yearOf(release.Date)
	}
	genre := e.genre(ctx, group.ID)
	song.Title = cleanDisplay(recording.Title)
	song.Artist = creditName(recording.ArtistCredit)
	song.Album = cleanDisplay(group.Title)
	if song.Album == "" {
		song.Album = cleanDisplay(release.Title)
	}
	song.Year = year
	song.Genre = genre
	song.MetadataSource = method
	song.MetadataConfidence = confidence
	song.RecordingMBID = recording.ID
	song.ReleaseGroupMBID = group.ID
	song.ReleaseMBID = release.ID
	if track := e.trackNumber(ctx, release.ID, recording.ID); track > 0 {
		song.TrackNumber = track
	}
	if err = e.store.UpdateSongMetadata(ctx, id, song); err != nil {
		e.fail(ctx, id, err.Error())
		return
	}
	art := e.cover(ctx, id, group.ID, release.ID)
	candidate, _ := json.Marshal(map[string]any{"artist": song.Artist, "title": song.Title, "album": song.Album, "genre": genre, "year": year, "confidence": confidence})
	status, message := "complete", "Identified by "+method
	if art == "" && attempts < 3 {
		status = "retry"
		message = "Metadata identified; canonical cover unavailable"
	} else if art == "" {
		message = "Identified; canonical cover unavailable after bounded retries"
	}
	_, _ = e.store.DB.ExecContext(ctx, `UPDATE song_enrichment SET status=?,match_score=?,recording_mbid=?,release_group_mbid=?,release_mbid=?,match_method=?,candidate_json=?,message=?,completed_at=CASE WHEN ?='complete' THEN CURRENT_TIMESTAMP END,next_attempt_at=CASE WHEN ?='retry' THEN datetime('now','+1 day') END,updated_at=CURRENT_TIMESTAMP WHERE song_id=?`, status, confidence, recording.ID, group.ID, release.ID, method, string(candidate), message, status, status, id)
	if e.broadcast != nil {
		e.broadcast("library.changed", map[string]any{"song_id": id, "enriched": true})
	}
}
func (e *Enricher) genre(ctx context.Context, groupID string) string {
	if groupID == "" {
		return ""
	}
	var g mbGroupLookup
	if e.mbGet(ctx, "release-group/"+url.PathEscape(groupID)+"?inc=genres&fmt=json", &g) != nil {
		return ""
	}
	sort.Slice(g.Genres, func(i, j int) bool { return g.Genres[i].Count > g.Genres[j].Count })
	if len(g.Genres) == 0 {
		return ""
	}
	names := []string{g.Genres[0].Name}
	if len(g.Genres) > 1 && g.Genres[1].Count > 0 && g.Genres[1].Count*2 >= g.Genres[0].Count {
		names = append(names, g.Genres[1].Name)
	}
	return strings.Join(names, " / ")
}
func (e *Enricher) trackNumber(ctx context.Context, releaseID, recordingID string) int {
	if releaseID == "" {
		return 0
	}
	var release mbRelease
	if e.mbGet(ctx, "release/"+url.PathEscape(releaseID)+"?inc=recordings&fmt=json", &release) != nil {
		return 0
	}
	for _, medium := range release.Media {
		for _, track := range medium.Tracks {
			if track.Recording.ID == recordingID {
				return track.Position
			}
		}
	}
	return 0
}
func (e *Enricher) cover(ctx context.Context, id int64, groupID, releaseID string) string {
	for _, pair := range [][2]string{{"release-group", groupID}, {"release", releaseID}} {
		if pair[1] == "" {
			continue
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", "https://coverartarchive.org/"+pair[0]+"/"+pair[1]+"/front-500", nil)
		req.Header.Set("User-Agent", "ekkoPlayer/0.3")
		r, err := e.client.Do(req)
		if err != nil {
			continue
		}
		if r.StatusCode != 200 || r.ContentLength > 15<<20 {
			r.Body.Close()
			continue
		}
		dir := filepath.Join(e.cfg.ArtworkPath, "metadata")
		if os.MkdirAll(dir, 0755) != nil {
			r.Body.Close()
			return ""
		}
		key := groupID
		if key == "" {
			key = releaseID
		}
		dst := filepath.Join(dir, key+".jpg")
		f, err := os.CreateTemp(dir, ".cover-*")
		if err != nil {
			r.Body.Close()
			return ""
		}
		tmp := f.Name()
		_, copyErr := io.Copy(f, io.LimitReader(r.Body, 15<<20))
		r.Body.Close()
		closeErr := f.Close()
		if copyErr == nil {
			copyErr = closeErr
		}
		if copyErr == nil {
			copyErr = os.Chmod(tmp, 0644)
		}
		if copyErr == nil {
			copyErr = os.Rename(tmp, dst)
		}
		if copyErr != nil {
			os.Remove(tmp)
			continue
		}
		path := "/art/metadata/" + key + ".jpg"
		_ = e.store.UpdateSongArtwork(ctx, id, path)
		return path
	}
	return ""
}
func (e *Enricher) fail(ctx context.Context, id int64, msg string) {
	var attempts int
	_ = e.store.DB.QueryRowContext(ctx, "SELECT attempts FROM song_enrichment WHERE song_id=?", id).Scan(&attempts)
	if attempts >= 3 {
		e.unmatched(ctx, id, msg)
		return
	}
	_, _ = e.store.DB.ExecContext(ctx, `UPDATE song_enrichment SET status='retry',message=?,next_attempt_at=datetime('now','+1 day'),updated_at=CURRENT_TIMESTAMP WHERE song_id=?`, msg, id)
	slog.Warn("metadata enrichment retry", "song_id", id, "error", msg)
}
func (e *Enricher) unmatched(ctx context.Context, id int64, msg string) {
	_, _ = e.store.DB.ExecContext(ctx, `UPDATE song_enrichment SET status='unmatched',message=?,updated_at=CURRENT_TIMESTAMP WHERE song_id=?`, msg, id)
}
func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
