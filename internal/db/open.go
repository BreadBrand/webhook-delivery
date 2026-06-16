package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)",
		path,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := runSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// testDB opens an in-memory SQLite instance. Only called from _test.go files.
func testDB() (*sql.DB, error) {
	uri := fmt.Sprintf("file:testdb_%d?mode=memory&cache=shared&_pragma=foreign_keys(ON)", time.Now().UnixNano())
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return nil, err
	}
	if err := runSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
