package store

import (
	"database/sql"
	"errors"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type MCMessage struct {
	ID        string    `json:"id"`
	ServerID  string    `json:"server_id"`
	Type      string    `json:"type"`
	Text      string    `json:"text"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
}

type Heartbeat struct {
	ServerID      string    `json:"server_id"`
	OnlinePlayers int       `json:"online_players"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Ping() error {
	return s.db.Ping()
}

func (s *Store) migrate() error {
	stmts := []string{
		`create table if not exists seen_traces (
			trace_id text primary key,
			source text not null,
			created_at datetime not null
		)`,
		`create table if not exists mc_queue (
			id text primary key,
			server_id text not null,
			type text not null,
			text text not null,
			source text not null,
			acked integer not null default 0,
			created_at datetime not null
		)`,
		`create table if not exists heartbeats (
			server_id text primary key,
			online_players integer not null,
			updated_at datetime not null
		)`,
		`create table if not exists event_log (
			id integer primary key autoincrement,
			level text not null,
			message text not null,
			created_at datetime not null
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) MarkTrace(traceID, source string) (bool, error) {
	if traceID == "" {
		return true, nil
	}
	res, err := s.db.Exec(
		`insert or ignore into seen_traces(trace_id, source, created_at) values(?, ?, ?)`,
		traceID,
		source,
		time.Now().UTC(),
	)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (s *Store) Enqueue(msg MCMessage) error {
	if msg.ID == "" {
		return errors.New("message id is required")
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.Exec(
		`insert or ignore into mc_queue(id, server_id, type, text, source, created_at) values(?, ?, ?, ?, ?, ?)`,
		msg.ID,
		msg.ServerID,
		msg.Type,
		msg.Text,
		msg.Source,
		msg.CreatedAt,
	)
	return err
}

func (s *Store) Pull(serverID string, limit int) ([]MCMessage, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.Query(
		`select id, server_id, type, text, source, created_at
		 from mc_queue
		 where server_id = ? and acked = 0
		 order by created_at asc
		 limit ?`,
		serverID,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MCMessage
	for rows.Next() {
		var msg MCMessage
		if err := rows.Scan(&msg.ID, &msg.ServerID, &msg.Type, &msg.Text, &msg.Source, &msg.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}

func (s *Store) Ack(serverID string, ids []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, err := tx.Exec(`update mc_queue set acked = 1 where server_id = ? and id = ?`, serverID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SaveHeartbeat(serverID string, onlinePlayers int) error {
	_, err := s.db.Exec(
		`insert into heartbeats(server_id, online_players, updated_at)
		 values(?, ?, ?)
		 on conflict(server_id) do update set
		   online_players = excluded.online_players,
		   updated_at = excluded.updated_at`,
		serverID,
		onlinePlayers,
		time.Now().UTC(),
	)
	return err
}

func (s *Store) LastHeartbeat(serverID string) (Heartbeat, bool, error) {
	var hb Heartbeat
	err := s.db.QueryRow(
		`select server_id, online_players, updated_at from heartbeats where server_id = ?`,
		serverID,
	).Scan(&hb.ServerID, &hb.OnlinePlayers, &hb.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Heartbeat{}, false, nil
	}
	if err != nil {
		return Heartbeat{}, false, err
	}
	return hb, true, nil
}

func (s *Store) Log(level, message string) {
	_, _ = s.db.Exec(
		`insert into event_log(level, message, created_at) values(?, ?, ?)`,
		level,
		message,
		time.Now().UTC(),
	)
}

func (s *Store) RecentLogs(limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 200 {
		limit = 80
	}
	rows, err := s.db.Query(
		`select level, message, created_at from event_log order by id desc limit ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var level, message string
		var createdAt time.Time
		if err := rows.Scan(&level, &message, &createdAt); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"level":      level,
			"message":    message,
			"created_at": createdAt,
		})
	}
	return out, rows.Err()
}
