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

// Service 协调配置、Feed 获取、去重存储和 Markdown 归档几个组件。
type Service struct {
	Config  config.Config    // 订阅和运行时配置。
	Fetcher *fetcher.Fetcher // 负责下载、解析 Feed。
	Store   *store.Store     // 负责查询去重状态和保存条目记录。
	Writer  *archive.Writer  // 负责将新条目写为 Vault 中的 Markdown 笔记。
	Logger  *log.Logger      // 同步过程使用的日志输出器。
}

// SyncAll 依次同步所有已启用订阅。单个订阅失败不会阻止后续订阅，最后返回遇到的第一个错误。
func (s *Service) SyncAll(ctx context.Context) error { //声明这是service结构体的SyncAll方法，接收者为指针s作为实例
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

// SyncOne 同步一个订阅并返回本轮新归档的条目数；已存在的 GUID 会跳过。
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
		// 先写 Markdown，再登记数据库：数据库中的记录代表对应笔记已经成功生成。
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
