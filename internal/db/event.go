package db

import "database/sql"

type EventStore struct{ db *sql.DB }
