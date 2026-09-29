package model

import "time"

// Item 是项目内部统一的订阅条目结构，由 Fetcher 填充，再传给归档器和存储层。
type Item struct {
	SubscriptionID string    // 产生该条目的订阅 ID，用于数据库去重范围。
	Source         string    // 来源平台或分类名称。
	FeedTitle      string    // Feed 本身的标题。
	GUID           string    // Feed 条目稳定标识；缺失时由 Fetcher 回退生成。
	Title          string    // 条目标题。
	Link           string    // 原文链接。
	Author         string    // 作者名称或邮箱。
	Description    string    // Feed 提供的摘要，可在正文缺失时作为回退内容。
	Content        string    // Feed 提供的正文内容。
	PublishedAt    time.Time // 发布时间；零值表示 Feed 未提供可用时间。
	UpdatedAt      time.Time // 更新时间；缺失时沿用发布时间。
}
