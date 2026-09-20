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

type Fetcher struct {
	Client    *http.Client
	UserAgent string
	Parser    *gofeed.Parser
}

func New(userAgent string) *Fetcher {
	return &Fetcher{
		Client:    &http.Client{Timeout: 45 * time.Second},
		UserAgent: userAgent,
		Parser:    gofeed.NewParser(),
	}
}

func (f *Fetcher) Fetch(ctx context.Context, feedURL string) ([]model.Item, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", f.UserAgent)
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", feedURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch %s: http %s", feedURL, resp.Status)
	}

	feed, err := f.Parser.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", feedURL, err)
	}
	items := make([]model.Item, 0, len(feed.Items))
	for _, sourceItem := range feed.Items {
		guid := strings.TrimSpace(sourceItem.GUID)
		if guid == "" {
			guid = strings.TrimSpace(sourceItem.Link)
		}
		if guid == "" {
			guid = strings.TrimSpace(sourceItem.Title) + "\x00" + sourceItem.Published
		}
		if guid == "" {
			continue
		}

		publishedAt := time.Time{}
		if sourceItem.PublishedParsed != nil {
			publishedAt = sourceItem.PublishedParsed.UTC()
		} else if sourceItem.UpdatedParsed != nil {
			publishedAt = sourceItem.UpdatedParsed.UTC()
		}
		updatedAt := publishedAt
		if sourceItem.UpdatedParsed != nil {
			updatedAt = sourceItem.UpdatedParsed.UTC()
		}

		author := ""
		if sourceItem.Author != nil {
			author = sourceItem.Author.Name
			if author == "" {
				author = sourceItem.Author.Email
			}
		}
		if author == "" && len(sourceItem.Authors) > 0 && sourceItem.Authors[0] != nil {
			author = sourceItem.Authors[0].Name
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
