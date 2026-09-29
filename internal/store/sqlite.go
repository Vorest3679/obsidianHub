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

// Store 封装条目去重和记录所需的 SQLite 操作。
type Store struct {
	db *sql.DB // database/sql 连接池句柄；底层 SQLite 驱动由 go-sqlite3 注册。
}

// Open 创建 SQLite 数据库目录和连接池，并确保所需表、索引已经创建。
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

// Close 关闭 database/sql 管理的连接池；Store 的调用方负责在生命周期结束时调用它。
func (s *Store) Close() error { return s.db.Close() }

// migrate 创建条目表和查询索引。IF NOT EXISTS 允许程序每次启动时安全地执行初始化。
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

// Has 查询指定订阅下是否已经记录该 GUID，用于同步时跳过重复条目。
func (s *Store) Has(subscriptionID, guid string) (bool, error) {
	// QueryRow 表示只读取一个结果行；Scan 把 COUNT 查询结果写入 count。
	var count int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM items WHERE subscription_id = ? AND guid = ?`, subscriptionID, guid).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check item: %w", err)
	}
	return count > 0, nil
}

// Insert 保存条目元数据和生成的笔记路径；问号占位符把数据作为参数传给驱动。
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

// timeString 将有效时间以 UTC RFC3339 格式存储；time.Time 的零值保存为空字符串。
func timeString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
