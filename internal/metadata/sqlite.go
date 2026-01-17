package metadata

import (
	"database/sql"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type SqliteStorage struct {
	db *sqlx.DB
}

func NewSqliteStorage(dbPath string) (*SqliteStorage, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sqlx.Connect("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database at %s: %w", dbPath, err)
	}

	s := &SqliteStorage{db: db}
	if err := s.initSchema(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *SqliteStorage) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS files (
		path TEXT PRIMARY KEY COLLATE NOCASE,
		remote_name TEXT UNIQUE NOT NULL,
		is_dir BOOLEAN NOT NULL,
		size INTEGER NOT NULL,
		fek BLOB,
		salt BLOB,
		updated_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_remote_name ON files(remote_name);
	`
	_, err := s.db.Exec(schema)
	return err
}

func (s *SqliteStorage) Get(path string) (*FileMeta, error) {
	var meta FileMeta
	err := s.db.Get(&meta, "SELECT * FROM files WHERE path = ?", path)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &meta, err
}

func (s *SqliteStorage) GetByRemoteName(remoteName string) (*FileMeta, error) {
	var meta FileMeta
	err := s.db.Get(&meta, "SELECT * FROM files WHERE remote_name = ?", remoteName)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &meta, err
}

func (s *SqliteStorage) Save(meta *FileMeta) error {
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
	_, err := s.db.NamedExec(query, meta)
	return err
}

func (s *SqliteStorage) Delete(path string) error {
	_, err := s.db.Exec("DELETE FROM files WHERE path = ?", path)
	return err
}

func (s *SqliteStorage) ReadDir(p string) ([]FileMeta, error) {
	var files []FileMeta
	var query string
	var args []interface{}

	// Match immediate children: (path LIKE prefix/%) AND (path NOT LIKE prefix/%/%)
	if p == "/" {
		query = "SELECT * FROM files WHERE path LIKE '/%' AND path NOT LIKE '/%/%'"
		args = []interface{}{}
	} else {
		prefix := p
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		query = "SELECT * FROM files WHERE path LIKE ? AND path NOT LIKE ?"
		args = []interface{}{prefix + "%", prefix + "%/%"}
	}

	err := s.db.Select(&files, query, args...)
	return files, err
}

func (s *SqliteStorage) RemoveAll(prefix string) error {
	var query string
	var args []interface{}

	if prefix == "/" {
		query = "DELETE FROM files"
		args = []interface{}{}
	} else {
		query = "DELETE FROM files WHERE path = ? OR path LIKE ? ESCAPE '\\'"
		args = []interface{}{prefix, prefix + "/%"}
	}

	_, err := s.db.Exec(query, args...)
	return err
}

func (s *SqliteStorage) Rename(oldPath, newPath string) error {
	// Recursive rename for SQLite requires updating all paths starting with oldPath
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Get all items to rename
	var items []FileMeta
	var query string
	var args []interface{}
	if oldPath == "/" {
		query = "SELECT * FROM files"
	} else {
		query = "SELECT * FROM files WHERE path = ? OR path LIKE ? ESCAPE '\\'"
		args = []interface{}{oldPath, oldPath + "/%"}
	}
	if err := tx.Select(&items, query, args...); err != nil {
		return err
	}

	// 2. Update each item
	for _, item := range items {
		rel := strings.TrimPrefix(item.Path, oldPath)
		targetPath := path.Join(newPath, rel)

		_, err := tx.Exec("UPDATE files SET path = ? WHERE path = ?", targetPath, item.Path)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SqliteStorage) Close() error {
	return s.db.Close()
}
