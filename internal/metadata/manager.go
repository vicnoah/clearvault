package metadata

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type FileMeta struct {
	ID         int64     `db:"id"`
	Path       string    `db:"path"`
	RemoteName string    `db:"remote_name"`
	Size       int64     `db:"size"`
	IsDir      bool      `db:"is_dir"`
	FEK        []byte    `db:"fek"`
	Salt       []byte    `db:"salt"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

type Manager struct {
	db *sqlx.DB
}

func NewManager(dbPath string) (*Manager, error) {
	// Ensure the directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sqlx.Connect("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database at %s: %w", dbPath, err)
	}

	m := &Manager{db: db}
	if err := m.initSchema(); err != nil {
		return nil, err
	}

	return m, nil
}

func (m *Manager) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		path TEXT UNIQUE NOT NULL,
		remote_name TEXT UNIQUE NOT NULL,
		size INTEGER NOT NULL,
		is_dir BOOLEAN NOT NULL,
		fek BLOB,
		salt BLOB,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_path ON files(path);
	CREATE INDEX IF NOT EXISTS idx_remote_name ON files(remote_name);
	`
	_, err := m.db.Exec(schema)
	return err
}

func (m *Manager) GetByPath(path string) (*FileMeta, error) {
	var meta FileMeta
	err := m.db.Get(&meta, "SELECT * FROM files WHERE path = ?", path)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &meta, err
}

func (m *Manager) GetByRemoteName(remoteName string) (*FileMeta, error) {
	var meta FileMeta
	err := m.db.Get(&meta, "SELECT * FROM files WHERE remote_name = ?", remoteName)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &meta, err
}

func (m *Manager) Save(meta *FileMeta) error {
	query := `
	INSERT INTO files (path, remote_name, size, is_dir, fek, salt, updated_at)
	VALUES (:path, :remote_name, :size, :is_dir, :fek, :salt, :updated_at)
	ON CONFLICT(path) DO UPDATE SET
		remote_name = excluded.remote_name,
		size = excluded.size,
		is_dir = excluded.is_dir,
		fek = excluded.fek,
		salt = excluded.salt,
		updated_at = excluded.updated_at
	`
	_, err := m.db.NamedExec(query, meta)
	return err
}

func (m *Manager) Delete(path string) error {
	_, err := m.db.Exec("DELETE FROM files WHERE path = ?", path)
	return err
}

func (m *Manager) ListByPrefix(prefix string) ([]FileMeta, error) {
	var files []FileMeta
	err := m.db.Select(&files, "SELECT * FROM files WHERE path LIKE ? ESCAPE '\\'", prefix+"%")
	return files, err
}

func (m *Manager) Close() error {
	return m.db.Close()
}
