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

	xhtml "golang.org/x/net/html"

	"obsidianhub/internal/config"
	"obsidianhub/internal/model"
)

// unsafeFilename 匹配路径分隔符、常见文件名禁用字符和控制字符。
var unsafeFilename = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]+`)

// spaces 和 hyphens 用来把连续空白、连续连字符统一成便于阅读的单个连字符。
var spaces = regexp.MustCompile(`\s+`)
var hyphens = regexp.MustCompile(`-+`)
var highlightedCodeBlock = regexp.MustCompile(`(?is)<div\b[^>]*\bclass\s*=\s*["'][^"']*\bhighlight\b[^"']*["'][^>]*>\s*<pre\b[^>]*>.*?</pre\s*>\s*</div\s*>`)
var preformattedBlock = regexp.MustCompile(`(?is)<pre\b[^>]*>.*?</pre\s*>`)
var languageClass = regexp.MustCompile(`(?i)(?:^|\s)(?:language|lang)-([a-z0-9_+.-]+)(?:\s|$)`)
var backtickSequence = regexp.MustCompile("`+")

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
	// Feed 中的高亮代码通常是 Pygments 生成的 HTML；先提取代码文本并
	// 转成 Markdown 围栏，再解码正文中的 HTML 实体。
	b.WriteString(html.UnescapeString(markdownCodeBlocks(content)))
	b.WriteString("\n")
	return b.String()
}

// markdownCodeBlocks 把高亮容器和普通 pre 元素转换成 Obsidian 可渲染的代码围栏。
func markdownCodeBlocks(content string) string {
	content = replaceCodeBlocks(content, highlightedCodeBlock)
	return replaceCodeBlocks(content, preformattedBlock)
}

func replaceCodeBlocks(content string, pattern *regexp.Regexp) string {
	return pattern.ReplaceAllStringFunc(content, func(block string) string {
		code, language, ok := extractCodeBlock(block)
		if !ok {
			return block
		}
		fence := codeFence(code)
		markdown := fence + language + "\n" + code
		if !strings.HasSuffix(code, "\n") {
			markdown += "\n"
		}
		markdown += fence
		// The whole body is unescaped after this conversion. Escape the generated
		// Markdown once so code entities such as &lt; remain literal code text.
		return "\n\n" + html.EscapeString(markdown) + "\n\n"
	})
}

func extractCodeBlock(block string) (code, language string, ok bool) {
	root, err := xhtml.Parse(strings.NewReader(block))
	if err != nil {
		return "", "", false
	}
	pre := findElement(root, "pre")
	if pre == nil {
		return "", "", false
	}
	// 默认从 <pre> 节点收集文本；没有嵌套 <code> 时也能处理普通预格式化内容。
	content := pre
	if codeNode := findElement(pre, "code"); codeNode != nil {
		// <pre><code> 是常见代码块结构，优先读取 <code>，避免带入外围节点内容。
		content = codeNode
		for _, attr := range codeNode.Attr {
			// 语言标识通常写在 class 中，例如 "language-python"。
			if attr.Key != "class" {
				continue
			}
			// 正则的第 2 组是语言名；找不到时保留空字符串，生成无语言标记的围栏。
			if match := languageClass.FindStringSubmatch(attr.Val); len(match) == 2 {
				language = match[1]
			}
		}
	}
	// 高亮 HTML 的代码字符分散在多个 <span> 中，递归拼接文本节点以还原原始代码。
	var text strings.Builder
	appendText(content, &text)
	return text.String(), language, true
}

func findElement(node *xhtml.Node, name string) *xhtml.Node {
	if node.Type == xhtml.ElementNode && node.Data == name {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findElement(child, name); found != nil {
			return found
		}
	}
	return nil
}

func appendText(node *xhtml.Node, text *strings.Builder) {
	if node.Type == xhtml.TextNode {
		text.WriteString(node.Data)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		appendText(child, text)
	}
}

// codeFence 生成 Markdown 代码围栏：至少 3 个反引号，并比代码中的最长连续反引号多 1 个，避免围栏提前闭合。
func codeFence(code string) string {
	longest := 2 // 初始值 2 保证最终围栏至少有 3 个反引号。
	for _, match := range backtickSequence.FindAllString(code, -1) {
		if len(match) > longest {
			// 记录代码中最长的一段连续反引号，供最终围栏长度参考。
			longest = len(match)
		}
	}
	return strings.Repeat("`", longest+1)
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
