package api

import (
	"os"
	"path/filepath"
	"testing"

	"ekkoplayer/internal/config"
)

func TestResolveMusicFolderRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Albums"), 0755); err != nil {
		t.Fatal(err)
	}
	s := &Server{cfg: config.Config{MusicPath: root}}
	got, err := s.resolveMusicFolder("Albums", true)
	if err != nil || got != filepath.Join(root, "Albums") {
		t.Fatalf("resolve got %q, %v", got, err)
	}
	for _, path := range []string{"../outside", "Albums/../../outside", filepath.Join(string(filepath.Separator), "tmp")} {
		if _, err := s.resolveMusicFolder(path, false); err == nil {
			t.Fatalf("expected %q to be rejected", path)
		}
	}
}

func TestRemoveEmptyMusicParentsStopsAtRootAndNonEmptyFolder(t *testing.T) {
	root := t.TempDir()
	emptyLeaf := filepath.Join(root, "Artist", "Album")
	if err := os.MkdirAll(emptyLeaf, 0755); err != nil {
		t.Fatal(err)
	}
	removeEmptyMusicParents(emptyLeaf, root)
	if _, err := os.Stat(filepath.Join(root, "Artist")); !os.IsNotExist(err) {
		t.Fatalf("empty parent was not removed: %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("music root was removed: %v", err)
	}

	nonEmpty := filepath.Join(root, "Keep", "Child")
	if err := os.MkdirAll(nonEmpty, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Keep", "other.flac"), []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	removeEmptyMusicParents(nonEmpty, root)
	if _, err := os.Stat(filepath.Join(root, "Keep")); err != nil {
		t.Fatalf("non-empty parent was removed: %v", err)
	}
	if _, err := os.Stat(nonEmpty); !os.IsNotExist(err) {
		t.Fatalf("empty leaf was not removed: %v", err)
	}
}
