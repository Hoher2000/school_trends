package collector

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type RutubeVideo struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Thumbnail   string `json:"thumbnail_url"`
	PlayerURL   string `json:"player_url"`
}

type RutubeSearchResponse struct {
	Results []RutubeVideo `json:"results"`
}

// FetchRutubeVideo ищет видео на Rutube и скачивает первое найденное.
func FetchRutubeVideo(query string) (string, error) {
	u, _ := url.Parse("https://rutube.ru/api/search/videos/")
	q := u.Query()
	q.Set("query", query)
	q.Set("limit", "5")
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		return "", fmt.Errorf("rutube search: %w", err)
	}
	defer resp.Body.Close()

	var searchResp RutubeSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return "", fmt.Errorf("rutube decode: %w", err)
	}
	if len(searchResp.Results) == 0 {
		return "", fmt.Errorf("rutube: нет результатов")
	}

	// Rutube не даёт прямых ссылок на mp4. Сохраняем ссылку на плеер в текстовый файл.
	video := searchResp.Results[0]
	os.MkdirAll("backgrounds", 0755)
	filename := filepath.Join("backgrounds", fmt.Sprintf("rutube_%s_%d.txt", sanitizeFilename(video.Title), video.ID))
	f, err := os.Create(filename)
	if err != nil {
		return "", fmt.Errorf("создать файл: %w", err)
	}
	defer f.Close()
	fmt.Fprintf(f, "%s\n%s\n", video.PlayerURL, video.Title)
	return filename, nil
}

func sanitizeFilename(s string) string {
	repl := []string{" ", "/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	res := s
	for _, ch := range repl {
		res = strings.ReplaceAll(res, ch, "_")
	}
	return res
}
