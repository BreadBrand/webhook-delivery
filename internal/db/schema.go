package db

import (
	_ "embed"
	"database/sql"
	"fmt"
)

//go:embed schema.sql
var schemaSQL string

func runSchema(db *sql.DB) error {
	if _, err := db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("run schema: %w", err)
	}
	return nil
}
