package collector

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"
)

// PipedVideo представляет структуру ответа от Piped API.
type PipedVideo struct {
	URL          string `json:"url"`
	Title        string `json:"title"`
	UploaderName string `json:"uploaderName"`
	Duration     int    `json:"duration"`
}

type PipedSearchResponse struct {
	Items []PipedVideo `json:"items"`
}

// FetchPipedVideo ищет видео через Piped API и скачивает первое найденное.
func FetchPipedVideo(query string) (string, error) {
	instances := []string{
		"https://pipedapi.kavin.rocks",
		"https://pipedapi.tokhmi.xyz",
		"https://pipedapi.moomoo.me",
	}

	for _, baseURL := range instances {
		log.Printf("[Piped] Пробую инстанс: %s", baseURL)
		video, err := searchAndDownloadPiped(baseURL, query)
		if err == nil {
			return video, nil
		}
		log.Printf("[Piped] Инстанс %s не сработал: %v", baseURL, err)
	}
	return "", fmt.Errorf("ни один инстанс Piped не ответил")
}

func searchAndDownloadPiped(baseURL, query string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	// 1. Поиск видео
	searchURL := fmt.Sprintf("%s/search?q=%s&filter=videos", baseURL, url.QueryEscape(query))
	log.Printf("[Piped] Поиск: %s", searchURL)

	resp, err := client.Get(searchURL)
	if err != nil {
		return "", fmt.Errorf("поиск: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("статус %d: %s", resp.StatusCode, string(body))
	}

	var searchResp PipedSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return "", fmt.Errorf("парсинг: %w", err)
	}
	if len(searchResp.Items) == 0 {
		return "", fmt.Errorf("ничего не найдено")
	}

	// Берём первое видео
	video := searchResp.Items[0]
	log.Printf("[Piped] Найдено видео: %s", video.Title)

	// 2. Скачиваем видео через /streams/:videoId
	// Для простоты используем yt-dlp с полученным URL
	return DownloadWithYtDlp(video.URL)
}
