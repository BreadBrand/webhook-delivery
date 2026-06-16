package db_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/b2randon/webhook-delivery/internal/db"
)

func TestDBFilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	sqldb, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	sqldb.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("db file mode = %04o, want 0600", perm)
	}
}
