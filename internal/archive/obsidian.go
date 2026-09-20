package archive

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"obsidianhub/internal/config"
	"obsidianhub/internal/model"
)

var unsafeFilename = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]+`)
var spaces = regexp.MustCompile(`\s+`)
var hyphens = regexp.MustCompile(`-+`)

type Writer struct {
	VaultPath string
}

func New(vaultPath string) *Writer { return &Writer{VaultPath: vaultPath} }

func (w *Writer) Write(sub config.Subscription, item model.Item) (string, error) {
	when := item.PublishedAt
	if when.IsZero() {
		when = time.Now().UTC()
	}
	when = when.Local()
	folder := safeFolder(sub.Folder)
	if folder == "" {
		folder = filepath.Join("Subscriptions", safePart(sub.Source))
	}
	dir := filepath.Join(w.VaultPath, folder, when.Format("2006"), when.Format("01"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create vault folder: %w", err)
	}

	filename := when.Format("2006-01-02") + "-" + safePart(item.Title)
	if filename == when.Format("2006-01-02")+"-" {
		filename += "untitled"
	}
	h := sha1.Sum([]byte(item.GUID))
	filename += "-" + hex.EncodeToString(h[:])[:8] + ".md"
	path := filepath.Join(dir, filename)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("check note %q: %w", path, err)
	}

	body := renderMarkdown(sub, item, when)
	tmp, err := os.CreateTemp(dir, ".obsidianhub-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create note temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.WriteString(body); err != nil {
		tmp.Close()
		return "", fmt.Errorf("write note: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close note: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return "", fmt.Errorf("publish note: %w", err)
	}
	return path, nil
}

func renderMarkdown(sub config.Subscription, item model.Item, when time.Time) string {
	var b strings.Builder
	b.WriteString("---\n")
	fm("source", sub.Source, &b)
	fm("subscription", sub.ID, &b)
	fm("title", item.Title, &b)
	fm("author", item.Author, &b)
	fm("published", when.Format(time.RFC3339), &b)
	fm("url", item.Link, &b)
	b.WriteString("tags:\n")
	for _, tag := range sub.Tags {
		if strings.TrimSpace(tag) != "" {
			fmt.Fprintf(&b, "  - %s\n", yamlScalar(tag))
		}
	}
	b.WriteString("---\n\n")
	title := item.Title
	if title == "" {
		title = "未命名条目"
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	if item.Link != "" {
		fmt.Fprintf(&b, "> 原文：[%s](%s)\n\n", item.Link, item.Link)
	}
	content := strings.TrimSpace(item.Content)
	if content == "" {
		content = strings.TrimSpace(item.Description)
	}
	if content == "" {
		content = "（该 RSS 条目没有正文摘要。）"
	}
	b.WriteString("## 内容\n\n")
	b.WriteString(html.UnescapeString(content))
	b.WriteString("\n")
	return b.String()
}

func fm(key, value string, b *strings.Builder) {
	fmt.Fprintf(b, "%s: %s\n", key, yamlScalar(value))
}

func yamlScalar(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	value = strings.ReplaceAll(value, "\n", " ")
	return `"` + value + `"`
}

func safePart(value string) string {
	value = unsafeFilename.ReplaceAllString(value, "-")
	value = spaces.ReplaceAllString(strings.TrimSpace(value), "-")
	value = hyphens.ReplaceAllString(value, "-")
	value = strings.Trim(value, ".-")
	if value == "" {
		return "untitled"
	}
	if len([]rune(value)) > 100 {
		value = string([]rune(value)[:100])
	}
	return value
}

func safeFolder(value string) string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '\\' })
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "." || part == ".." {
			continue
		}
		part = safePart(part)
		if part != "" {
			clean = append(clean, part)
		}
	}
	return filepath.Join(clean...)
}
