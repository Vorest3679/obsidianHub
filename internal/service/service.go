package service

import (
	"context"
	"fmt"
	"log"

	"obsidianhub/internal/archive"
	"obsidianhub/internal/config"
	"obsidianhub/internal/fetcher"
	"obsidianhub/internal/store"
)

type Service struct {
	Config  config.Config
	Fetcher *fetcher.Fetcher
	Store   *store.Store
	Writer  *archive.Writer
	Logger  *log.Logger
}

func (s *Service) SyncAll(ctx context.Context) error {
	var firstErr error
	for _, sub := range s.Config.Subscriptions {
		if !sub.Enabled {
			continue
		}
		if _, err := s.SyncOne(ctx, sub); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			s.Logger.Printf("sync failed: %s: %v", sub.Name, err)
		}
	}
	return firstErr
}

func (s *Service) SyncOne(ctx context.Context, sub config.Subscription) (int, error) {
	feedURL, err := config.ResolveURL(s.Config.RSSHubBaseURL, sub.URL)
	if err != nil {
		return 0, err
	}
	items, err := s.Fetcher.Fetch(ctx, feedURL)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", sub.Name, err)
	}
	count := 0
	for _, item := range items {
		item.SubscriptionID = sub.ID
		item.Source = sub.Source
		exists, err := s.Store.Has(sub.ID, item.GUID)
		if err != nil {
			return count, err
		}
		if exists {
			continue
		}
		notePath, err := s.Writer.Write(sub, item)
		if err != nil {
			return count, fmt.Errorf("archive %s: %w", sub.Name, err)
		}
		if err := s.Store.Insert(item, notePath); err != nil {
			return count, fmt.Errorf("record %s: %w", sub.Name, err)
		}
		count++
		s.Logger.Printf("archived: [%s] %s", sub.Name, item.Title)
	}
	return count, nil
}
