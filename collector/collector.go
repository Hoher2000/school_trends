package collector

import (
	"log"
	"sort"
	"sync"
	"time"
)

// Article — единица собранного контента.
type Article struct {
	Title       string
	Description string
	Link        string
	Source      string // "google_news", "youtube"
	Published   time.Time
}

// Fetcher умеет получать список статей.
type Fetcher interface {
	Fetch() ([]Article, error)
}

// CollectParams задаёт параметры сбора.
type CollectParams struct {
	NewsQueries    []string      // запросы для Google News
	YouTubeApiKey  string        // ключ API YouTube (если нужен поиск по YouTube)
	YouTubeQueries []string      // запросы для YouTube (если пусто – пропускаем)
	MaxArticles    int           // лимит на выходе
	Dedup          *Deduplicator // может быть nil (без дедупликации)
	// Больше НЕ используем TrendsGeo – удалено
}

// CollectTrends выполняет полный сбор из всех заданных источников.
func CollectTrends(params CollectParams) ([]Article, error) {
	var fetchers []Fetcher

	// 1. Google News по каждому запросу
	for _, q := range params.NewsQueries {
		fetchers = append(fetchers, &GoogleNewsFetcher{Query: q, Lang: "ru", Country: "RU"})
	}

	// 2. YouTube – если задан ключ и есть запросы
	if params.YouTubeApiKey != "" && len(params.YouTubeQueries) > 0 {
		for _, q := range params.YouTubeQueries {
			fetchers = append(fetchers, &YouTubeFetcher{
				ApiKey:     params.YouTubeApiKey,
				Query:      q,
				MaxResults: 10, // до 10 видео на запрос (можно вынести в параметры)
			})
		}
	}

	// Параллельный сбор
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		allArts []Article
	)
	for _, fetcher := range fetchers {
		wg.Add(1)
		go func(f Fetcher) {
			defer wg.Done()
			arts, err := f.Fetch()
			if err != nil {
				log.Printf("ERROR fetching: %v", err)
				return
			}
			mu.Lock()
			allArts = append(allArts, arts...)
			mu.Unlock()
		}(fetcher)
	}
	wg.Wait()

	// Фильтрация: убираем дубликаты по ссылке и уже опубликованные
	seen := make(map[string]bool)
	var filtered []Article
	for _, art := range allArts {
		if seen[art.Link] {
			continue
		}
		seen[art.Link] = true

		if params.Dedup != nil && params.Dedup.IsPublished(art.Link) {
			continue
		}
		filtered = append(filtered, art)
	}

	// Сортируем по дате (свежие сверху)
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Published.After(filtered[j].Published)
	})

	// Ограничиваем количество
	if params.MaxArticles > 0 && len(filtered) > params.MaxArticles {
		filtered = filtered[:params.MaxArticles]
	}
	return filtered, nil
}
