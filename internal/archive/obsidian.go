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

// unsafeFilename 匹配路径分隔符、常见文件名禁用字符和控制字符。
var unsafeFilename = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]+`)

// spaces 和 hyphens 用来把连续空白、连续连字符统一成便于阅读的单个连字符。
var spaces = regexp.MustCompile(`\s+`)
var hyphens = regexp.MustCompile(`-+`)

// Writer 把 Feed 条目保存到指定 Vault 的 Markdown 文件中。
type Writer struct {
	VaultPath string
}

// New 创建一个将笔记写入指定 Obsidian Vault 的 Writer。
func New(vaultPath string) *Writer { return &Writer{VaultPath: vaultPath} }

// Write 将一条 Feed 内容生成为 Markdown 笔记，并返回笔记的完整路径。
// 文件先写入同目录下的临时文件，再 Rename 为正式文件，避免中途失败留下半篇笔记。
func (w *Writer) Write(sub config.Subscription, item model.Item) (string, error) {
	when := item.PublishedAt
	if when.IsZero() {
		// Feed 没有发布时间时用当前 UTC 时间，保证后续目录和文件名仍可生成。
		when = time.Now().UTC()
	}
	// 归档目录按本地年月组织；写入 frontmatter 时仍保留明确的时区信息。
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
	// GUID 摘要只用于形成稳定、较短的文件名后缀，不用于安全或身份认证。
	h := sha1.Sum([]byte(item.GUID))
	filename += "-" + hex.EncodeToString(h[:])[:8] + ".md"
	path := filepath.Join(dir, filename)
	if _, err := os.Stat(path); err == nil {
		// 文件已存在时直接返回，避免重复覆盖用户可能编辑过的笔记。
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
	// 无论写入、关闭或改名是否成功，都尝试清理临时文件。
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

// renderMarkdown 生成包含 YAML frontmatter、标题、原文链接和正文的完整笔记。
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
		// 部分 Feed 只提供 Description；两者都为空时给出可读的占位说明。
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

// fm 按 frontmatter 的 key: value 格式写入一个字段，并统一通过 yamlScalar 转义值。
func fm(key, value string, b *strings.Builder) {
	fmt.Fprintf(b, "%s: %s\n", key, yamlScalar(value))
}

// yamlScalar 将值包在双引号中，并转义反斜杠、双引号和换行，避免破坏 YAML 结构。
func yamlScalar(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	value = strings.ReplaceAll(value, "\n", " ")
	return `"` + value + `"`
}

// safePart 把单个目录名或文件名片段中的非法字符替换掉，并限制名称长度。
func safePart(value string) string {
	value = unsafeFilename.ReplaceAllString(value, "-")
	value = spaces.ReplaceAllString(strings.TrimSpace(value), "-")
	value = hyphens.ReplaceAllString(value, "-")
	value = strings.Trim(value, ".-")
	if value == "" {
		return "untitled"
	}
	if len([]rune(value)) > 100 {
		// 按 rune 截断，避免把多字节 UTF-8 字符从中间切开。
		value = string([]rune(value)[:100])
	}
	return value
}

// safeFolder 按路径分隔符拆分用户配置，再逐段清理；丢弃 . 和 ..，避免目录穿越。
func safeFolder(value string) string {
	// 同时识别 Unix 和 Windows 分隔符，这样配置可以跨平台迁移。
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
