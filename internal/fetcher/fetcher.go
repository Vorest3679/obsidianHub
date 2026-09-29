package fetcher

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
	"obsidianhub/internal/model"
)

// Fetcher 持有可复用的 HTTP Client 和 Feed 解析器，避免每个条目重复初始化依赖。
type Fetcher struct {
	Client    *http.Client
	UserAgent string
	Parser    *gofeed.Parser
}

// New 创建 Feed 获取器。HTTP Client 会被复用，并设置单次请求的最长等待时间。
func New(userAgent string) *Fetcher {
	return &Fetcher{
		Client:    &http.Client{Timeout: 45 * time.Second},
		UserAgent: userAgent,
		Parser:    gofeed.NewParser(),
	}
}

// Fetch 下载并解析一个 Feed，将第三方解析器的条目转换为项目内部统一的 Item 类型。
func (f *Fetcher) Fetch(ctx context.Context, feedURL string) ([]model.Item, error) {
	// 把调用方 Context 绑定到请求上；同步被取消时，网络请求也会尽快结束。
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", f.UserAgent)
	// Client.Do 执行请求；无论状态码或解析结果如何，都要关闭响应体。
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", feedURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch %s: http %s", feedURL, resp.Status)
	}

	// gofeed 是第三方 Feed 解析库；标准库 net/http 只负责取得响应数据。
	feed, err := f.Parser.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", feedURL, err)
	}
	items := make([]model.Item, 0, len(feed.Items))
	for _, sourceItem := range feed.Items {
		guid := strings.TrimSpace(sourceItem.GUID)
		if guid == "" {
			// GUID 缺失时依次退回链接、标题加发布时间，尽量为条目建立稳定去重键。
			guid = strings.TrimSpace(sourceItem.Link)
		}
		if guid == "" {
			guid = strings.TrimSpace(sourceItem.Title) + "\x00" + sourceItem.Published
		}
		if guid == "" {
			continue
		}

		publishedAt := time.Time{}
		// 解析后的时间可能为空指针；优先发布时间，其次使用更新时间，并统一为 UTC。
		if sourceItem.PublishedParsed != nil {
			publishedAt = sourceItem.PublishedParsed.UTC()
		} else if sourceItem.UpdatedParsed != nil {
			publishedAt = sourceItem.UpdatedParsed.UTC()
		}
		updatedAt := publishedAt
		if sourceItem.UpdatedParsed != nil {
			// 有明确更新时间时用它覆盖默认值；否则 UpdatedAt 与 PublishedAt 相同。
			updatedAt = sourceItem.UpdatedParsed.UTC()
		}

		author := ""
		// gofeed 已弃用单数 Author 字段，统一从 Authors 中取第一个有效作者。
		for _, candidate := range sourceItem.Authors {
			if candidate == nil {
				continue
			}
			author = strings.TrimSpace(candidate.Name)
			if author == "" {
				author = strings.TrimSpace(candidate.Email)
			}
			if author != "" {
				break
			}
		}

		items = append(items, model.Item{
			FeedTitle:   feed.Title,
			GUID:        guid,
			Title:       strings.TrimSpace(sourceItem.Title),
			Link:        strings.TrimSpace(sourceItem.Link),
			Author:      strings.TrimSpace(author),
			Description: sourceItem.Description,
			Content:     sourceItem.Content,
			PublishedAt: publishedAt,
			UpdatedAt:   updatedAt,
		})
	}
	return items, nil
}
