package collector

import (
	"fmt"
	"net/url"
	"time"

	"github.com/mmcdole/gofeed"
)

type GoogleNewsFetcher struct {
	Query   string // "аниме", "roblox", "мемы"
	Lang    string // "ru"
	Country string // "RU"
}

func (f *GoogleNewsFetcher) Fetch() ([]Article, error) {
	// Формируем URL Google News RSS
	base := "https://news.google.com/rss/search"
	q := url.Values{}
	q.Set("q", f.Query)
	q.Set("hl", f.Lang)
	q.Set("gl", f.Country)
	q.Set("ceid", fmt.Sprintf("%s:%s", f.Country, f.Lang))
	feedURL := base + "?" + q.Encode()

	parser := gofeed.NewParser()
	feed, err := parser.ParseURL(feedURL)
	if err != nil {
		return nil, fmt.Errorf("parse feed: %w", err)
	}

	var articles []Article
	for _, item := range feed.Items {
		published := time.Now()
		if item.PublishedParsed != nil {
			published = *item.PublishedParsed
		}
		articles = append(articles, Article{
			Title:       item.Title,
			Description: item.Description,
			Link:        item.Link,
			Source:      "google_news",
			Published:   published,
		})
	}
	return articles, nil
}