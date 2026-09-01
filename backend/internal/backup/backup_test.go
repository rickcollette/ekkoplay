package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ekkoplayer/internal/config"
	"ekkoplayer/internal/db"
)

func TestRestoreRemovesWALSidecars(t *testing.T) {
	dir := t.TempDir()
	c := config.Defaults()
	c.DatabasePath = filepath.Join(dir, "player.db")
	c.BackupPath = filepath.Join(dir, "backup")
	s, err := db.Open(c.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.Stat(c.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := Create(context.Background(), c, s)
	s.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(c.DatabasePath+"-wal", []byte("stale"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(c.DatabasePath+"-shm", []byte("stale"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = Restore(filepath.Join(c.BackupPath, info.Name), c.DatabasePath); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(c.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != original.Mode().Perm() {
		t.Fatalf("restore mode=%v, want %v", st.Mode(), original.Mode())
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err = os.Stat(c.DatabasePath + suffix); !os.IsNotExist(err) {
			t.Fatalf("stale %s sidecar remains", suffix)
		}
	}
}
