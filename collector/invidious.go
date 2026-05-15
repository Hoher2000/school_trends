package collector

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type InvidiousVideo struct {
	Title         string `json:"title"`
	VideoID       string `json:"videoId"`
	LengthSeconds int    `json:"lengthSeconds"`
	FormatStreams []struct {
		URL    string `json:"url"`
		Itag   string `json:"itag"`
		Type   string `json:"type"`
		Quality string `json:"quality"`
	} `json:"formatStreams"`
}

func FetchInvidiousVideo(query string) (string, error) {
	instances := []string{
		"https://inv.nadeko.net",
		"https://yewtu.be",
		"https://invidious.nerdvpn.de",
		"https://yt.chocolatemoo53.com",
		"https://inv.thepixora.com",
	}

	for _, baseURL := range instances {
		log.Printf("[Invidious] Пробую инстанс: %s", baseURL)
		video, err := searchAndDownloadInvidious(baseURL, query)
		if err == nil {
			return video, nil
		}
		log.Printf("[Invidious] Инстанс %s не сработал: %v", baseURL, err)
	}
	return "", fmt.Errorf("ни один инстанс не ответил")
}

func searchAndDownloadInvidious(baseURL, query string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	// 1. Поиск видео
	searchURL := fmt.Sprintf("%s/api/v1/search?q=%s&type=video&sort=relevance", baseURL, url.QueryEscape(query))
	log.Printf("[Invidious] Поиск: %s", searchURL)

	resp, err := client.Get(searchURL)
	if err != nil {
		return "", fmt.Errorf("поиск: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("статус %d: %s", resp.StatusCode, string(body))
	}

    // Проверяем, что ответ действительно JSON
    bodyBytes, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", fmt.Errorf("чтение ответа: %w", err)
    }
    if !strings.HasPrefix(strings.TrimSpace(string(bodyBytes)), "[") {
        return "", fmt.Errorf("ответ не является JSON массивом (возможно, HTML): %s", string(bodyBytes[:100]))
    }

	var results []InvidiousVideo
	if err := json.Unmarshal(bodyBytes, &results); err != nil {
		return "", fmt.Errorf("парсинг: %w", err)
	}
	if len(results) == 0 {
		return "", fmt.Errorf("ничего не найдено")
	}

	video := results[0]
	log.Printf("[Invidious] Найдено видео: %s (ID: %s)", video.Title, video.VideoID)

	// 2. Выбор формата
	var downloadURL string
	for _, stream := range video.FormatStreams {
		if strings.Contains(stream.Type, "video/mp4") && (stream.Quality == "hd720" || stream.Quality == "medium" || stream.Quality == "sd") {
			downloadURL = stream.URL
			log.Printf("[Invidious] Выбран формат: %s (%s)", stream.Quality, stream.Type)
			break
		}
	}
	if downloadURL == "" {
		return "", fmt.Errorf("нет подходящего формата")
	}

	// 3. Скачивание
	log.Printf("[Invidious] Скачиваю: %s", downloadURL)
	videoResp, err := client.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("скачать: %w", err)
	}
	defer videoResp.Body.Close()

	os.MkdirAll("backgrounds", 0755)
	filename := filepath.Join("backgrounds", fmt.Sprintf("invidious_%s_%d.mp4", sanitizeFilename(query), time.Now().Unix()))
	file, err := os.Create(filename)
	if err != nil {
		return "", fmt.Errorf("создать файл: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, videoResp.Body)
	if err != nil {
		return "", fmt.Errorf("сохранить: %w", err)
	}

	log.Printf("[Invidious] Видео сохранено: %s", filename)
	return filename, nil
}