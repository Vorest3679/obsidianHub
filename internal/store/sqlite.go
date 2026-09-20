package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"obsidianhub/internal/model"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS items (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  subscription_id TEXT NOT NULL,
  guid TEXT NOT NULL,
  title TEXT NOT NULL,
  link TEXT,
  author TEXT,
  source TEXT,
  feed_title TEXT,
  published_at TEXT,
  updated_at TEXT,
  note_path TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(subscription_id, guid)
);
CREATE INDEX IF NOT EXISTS idx_items_subscription ON items(subscription_id);
CREATE INDEX IF NOT EXISTS idx_items_published ON items(published_at);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate sqlite: %w", err)
	}
	return nil
}

func (s *Store) Has(subscriptionID, guid string) (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM items WHERE subscription_id = ? AND guid = ?`, subscriptionID, guid).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check item: %w", err)
	}
	return count > 0, nil
}

func (s *Store) Insert(item model.Item, notePath string) error {
	_, err := s.db.Exec(`
INSERT INTO items (subscription_id, guid, title, link, author, source, feed_title, published_at, updated_at, note_path)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, item.SubscriptionID, item.GUID, item.Title, item.Link, item.Author, item.Source, item.FeedTitle, timeString(item.PublishedAt), timeString(item.UpdatedAt), notePath)
	if err != nil {
		return fmt.Errorf("insert item: %w", err)
	}
	return nil
}

func timeString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
