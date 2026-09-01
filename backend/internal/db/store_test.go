package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestProductionDatabaseStartsEmptyAndMigrates(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "player.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var songs, migrations, stations int
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM songs").Scan(&songs); err != nil {
		t.Fatal(err)
	}
	if songs != 0 {
		t.Fatalf("production database seeded %d songs", songs)
	}
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM radio_stations WHERE enabled=1").Scan(&stations); err != nil {
		t.Fatal(err)
	}
	if stations != 14 {
		t.Fatalf("clean appliance has %d enabled stations, want 14", stations)
	}
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&migrations); err != nil {
		t.Fatal(err)
	}
	if migrations != 16 {
		t.Fatalf("got %d migrations", migrations)
	}
}
func TestFolderPathUpdatesStayConsistent(t *testing.T) {
	s, err := OpenWithDemo(filepath.Join(t.TempDir(), "player.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	old := "/srv/ekkoplayer/music/demo"
	newPath := "/srv/ekkoplayer/music/organized/demo"
	if err = s.MoveSongPathPrefix(ctx, old, newPath); err != nil {
		t.Fatal(err)
	}
	song, err := s.Song(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if song.FilePath != newPath+"/01-demo.flac" {
		t.Fatalf("unexpected moved path %q", song.FilePath)
	}
	if err = s.UpdateSongPaths(ctx, map[int64]string{1: "/srv/ekkoplayer/music/singles/one.flac", 2: "/srv/ekkoplayer/music/singles/two.flac"}); err != nil {
		t.Fatal(err)
	}
	song, _ = s.Song(ctx, 2)
	if song.FilePath != "/srv/ekkoplayer/music/singles/two.flac" {
		t.Fatalf("unexpected song path %q", song.FilePath)
	}
}
func TestQueueReorderIsTransactional(t *testing.T) {
	s, err := OpenWithDemo(filepath.Join(t.TempDir(), "player.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	q, err := s.Queue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, len(q))
	for i := range q {
		ids[len(q)-1-i] = q[i].ID
	}
	if err = s.ReorderQueue(context.Background(), ids); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Queue(context.Background())
	if got[0].ID != ids[0] {
		t.Fatalf("queue not reordered")
	}
	if err = s.ReorderQueue(context.Background(), ids[:2]); err == nil {
		t.Fatal("expected incomplete reorder to fail")
	}
}
func TestReplaceQueueBuildsExactPlaylistQueue(t *testing.T) {
	s, err := OpenWithDemo(filepath.Join(t.TempDir(), "player.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err = s.ReplaceQueue(ctx, []int64{5, 2, 9}); err != nil {
		t.Fatal(err)
	}
	queue, err := s.Queue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 3 || queue[0].Song.ID != 5 || queue[1].Song.ID != 2 || queue[2].Song.ID != 9 {
		t.Fatalf("unexpected replacement queue: %+v", queue)
	}
	if err = s.RebuildShuffle(ctx); err != nil {
		t.Fatal(err)
	}
	shuffled, err := s.QueueForPlayback(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(shuffled) != 3 {
		t.Fatalf("shuffle lost playlist songs: %d", len(shuffled))
	}
}
func TestPlaylistReorderIsTransactional(t *testing.T) {
	s, err := OpenWithDemo(filepath.Join(t.TempDir(), "player.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	id, err := s.CreatePlaylist(ctx, "Reorder test")
	if err != nil {
		t.Fatal(err)
	}
	for _, songID := range []int64{1, 2, 3} {
		if err = s.AddPlaylistSong(ctx, id, songID); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.ReorderPlaylist(ctx, id, []int64{3, 1, 2}); err != nil {
		t.Fatal(err)
	}
	songs, err := s.PlaylistSongs(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if songs[0].ID != 3 || songs[1].ID != 1 || songs[2].ID != 2 {
		t.Fatalf("unexpected playlist order: %d, %d, %d", songs[0].ID, songs[1].ID, songs[2].ID)
	}
	if err = s.ReorderPlaylist(ctx, id, []int64{1, 2}); err == nil {
		t.Fatal("expected incomplete reorder to fail")
	}
	if err = s.AddPlaylistSongs(ctx, id, []int64{2, 3, 4, 4}); err != nil {
		t.Fatal(err)
	}
	songs, err = s.PlaylistSongs(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(songs) != 4 || songs[3].ID != 4 {
		t.Fatalf("bulk add should deduplicate existing and repeated songs, got %v", len(songs))
	}
}
func TestShuffleOrderPersistsAndContainsEveryItem(t *testing.T) {
	s, err := OpenWithDemo(filepath.Join(t.TempDir(), "player.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err = s.RebuildShuffle(ctx); err != nil {
		t.Fatal(err)
	}
	a, err := s.QueueForPlayback(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.QueueForPlayback(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 5 || len(b) != 5 {
		t.Fatalf("shuffle lengths %d/%d", len(a), len(b))
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			t.Fatal("shuffle order changed between reads")
		}
	}
}
func TestSongsPage(t *testing.T) {
	s, err := OpenWithDemo(filepath.Join(t.TempDir(), "player.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, err := s.SongsPage(context.Background(), "Midnight", 1, 5)
	if err != nil {
		t.Fatal(err)
	}
	if p.Total != 1 || len(p.Items) != 1 || p.Items[0].Title != "Midnight Circuit" {
		t.Fatalf("unexpected page: %+v", p)
	}
}
