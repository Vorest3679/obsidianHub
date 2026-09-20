package model

import "time"

type Item struct {
	SubscriptionID string
	Source         string
	FeedTitle      string
	GUID           string
	Title          string
	Link           string
	Author         string
	Description    string
	Content        string
	PublishedAt    time.Time
	UpdatedAt      time.Time
}
