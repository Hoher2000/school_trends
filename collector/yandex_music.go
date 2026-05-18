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
	"time"
)

type YandexMusicClient struct {
	token string
}

func NewYandexMusicClient(token string) *YandexMusicClient {
	return &YandexMusicClient{token: token}
}

// TrackInfo – информация о треке
type TrackInfo struct {
	ID    interface{} `json:"id"` // может быть float64 или string
	Title string      `json:"title"`
}

// SearchResponse – ответ поиска
type SearchResponse struct {
	Result struct {
		Tracks struct {
			Results []TrackInfo `json:"results"`
		} `json:"tracks"`
	} `json:"result"`
}

// DownloadInfo – ссылка на скачивание
type DownloadInfo struct {
	URL string `json:"url"`
}

// FetchTrackPreview ищет треки по запросу и скачивает превью первого найденного.
func (ymc *YandexMusicClient) FetchTrackPreview(query string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	// 1. Поиск треков
	searchURL := fmt.Sprintf("https://api.music.yandex.net:443/search?text=%s&type=track&page=0&nococrrect=false", url.QueryEscape(query))
	req, _ := http.NewRequest("GET", searchURL, nil)
	req.Header.Set("Authorization", "OAuth "+ymc.token)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("поиск: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("поиск: статус %d, тело: %s", resp.StatusCode, string(body))
	}

	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return "", fmt.Errorf("парсинг ответа: %w", err)
	}
	if len(searchResp.Result.Tracks.Results) == 0 {
		return "", fmt.Errorf("ничего не найдено")
	}

	track := searchResp.Result.Tracks.Results[0]
	trackID, err := convertIDToString(track.ID)
	if err != nil {
		return "", fmt.Errorf("преобразование ID: %w", err)
	}
	log.Printf("Найден трек: %s (ID: %s)", track.Title, trackID)

	// 2. Получение ссылки на превью
	downloadURL := fmt.Sprintf("https://api.music.yandex.net:443/tracks/%s/downloadInfo", trackID)
	req2, _ := http.NewRequest("GET", downloadURL, nil)
	req2.Header.Set("Authorization", "OAuth "+ymc.token)
	req2.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp2, err := client.Do(req2)
	if err != nil {
		return "", fmt.Errorf("получение ссылки на скачивание: %w", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp2.Body)
		return "", fmt.Errorf("скачивание: статус %d, тело: %s", resp2.StatusCode, string(body))
	}

	var downloadInfo []DownloadInfo
	if err := json.NewDecoder(resp2.Body).Decode(&downloadInfo); err != nil {
		return "", fmt.Errorf("парсинг ссылки: %w", err)
	}
	if len(downloadInfo) == 0 {
		return "", fmt.Errorf("нет ссылок на скачивание")
	}

	// 3. Скачиваем MP3
	mp3Resp, err := http.Get(downloadInfo[0].URL)
	if err != nil {
		return "", fmt.Errorf("скачивание mp3: %w", err)
	}
	defer mp3Resp.Body.Close()

	os.MkdirAll("backgrounds", 0755)
	filename := filepath.Join("backgrounds", fmt.Sprintf("music_%d.mp3", time.Now().Unix()))
	file, err := os.Create(filename)
	if err != nil {
		return "", fmt.Errorf("создание файла: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, mp3Resp.Body)
	if err != nil {
		return "", fmt.Errorf("сохранение mp3: %w", err)
	}

	return filename, nil
}

// convertIDToString преобразует ID трека в строку (убирает экспоненциальную запись)
func convertIDToString(id interface{}) (string, error) {
	switch v := id.(type) {
	case float64:
		return fmt.Sprintf("%.0f", v), nil
	case string:
		return v, nil
	default:
		return "", fmt.Errorf("неизвестный тип ID: %T", id)
	}
}
