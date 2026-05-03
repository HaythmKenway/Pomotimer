package main

import (
	"database/sql"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type LogEntry struct {
	ID        int
	StepName  string
	Duration  int // minutes
	StartTime time.Time
	EndTime   time.Time
}

func initDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "./tracker.db")
	if err != nil {
		return nil, err
	}

	createTableSQL := `CREATE TABLE IF NOT EXISTS logs (
		"id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
		"step_name" TEXT,
		"duration" INTEGER,
		"start_time" DATETIME,
		"end_time" DATETIME
	);`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		return nil, err
	}

	return db, nil
}

func logSession(db *sql.DB, step string, duration int, start time.Time) error {
	end := time.Now()
	_, err := db.Exec("INSERT INTO logs (step_name, duration, start_time, end_time) VALUES (?, ?, ?, ?)",
		step, duration, start, end)
	return err
}

func getHistory(db *sql.DB) []LogEntry {
	rows, err := db.Query("SELECT id, step_name, duration, start_time, end_time FROM logs ORDER BY id DESC LIMIT 20")
	if err != nil {
		return nil
	}
	defer rows.Close()

	var logs []LogEntry
	for rows.Next() {
		var l LogEntry
		err := rows.Scan(&l.ID, &l.StepName, &l.Duration, &l.StartTime, &l.EndTime)
		if err == nil {
			logs = append(logs, l)
		}
	}
	return logs
}
