package collector

import (
	"context"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

type YouTubeFetcher struct {
	ApiKey     string
	Query      string
	MaxResults int64
}

func (f *YouTubeFetcher) Fetch() ([]Article, error) {
	ctx := context.Background()

	// Создаём сервис с API-ключом
	service, err := youtube.NewService(ctx, option.WithAPIKey(f.ApiKey))
	if err != nil {
		return nil, err
	}

	// Ищем видео за последние 24 часа
	publishedAfter := time.Now().AddDate(0, 0, -1).Format(time.RFC3339)
	call := service.Search.List([]string{"id", "snippet"}).
		Q(f.Query).
		Type("video").
		MaxResults(f.MaxResults).
		Order("viewCount").
		PublishedAfter(publishedAfter).
		RelevanceLanguage("ru").
		SafeSearch("strict") // обязательный строгий поиск для детского контента

	response, err := call.Do()
	if err != nil {
		return nil, err
	}

	var articles []Article
	for _, item := range response.Items {
		snippet := item.Snippet
		published, _ := time.Parse(time.RFC3339, snippet.PublishedAt)
		link := "https://www.youtube.com/watch?v=" + item.Id.VideoId

		articles = append(articles, Article{
			Title:       snippet.Title,
			Description: snippet.Description,
			Link:        link,
			Source:      "youtube",
			Published:   published,
		})
	}
	return articles, nil
}

// FetchYouTubeBackground ищет видео на YouTube по ключевым словам и скачивает первое.
func FetchYouTubeBackground(query string) (string, error) {
	return DownloadWithYtDlp("ytsearch1:" + query)
}
