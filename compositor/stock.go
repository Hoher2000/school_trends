package compositor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const pexelsBaseURL = "https://api.pexels.com/videos/search"

// PexelsVideo представляет одно видео из ответа Pexels.
type PexelsVideo struct {
	ID         int    `json:"id"`
	URL        string `json:"url"`
	VideoFiles []struct {
		Link     string `json:"link"`
		Width    int    `json:"width"`
		Height   int    `json:"height"`
		FileType string `json:"file_type"`
	} `json:"video_files"`
}

// PexelsResponse – структура ответа API.
type PexelsResponse struct {
	Videos []PexelsVideo `json:"videos"`
}

// FetchStockVideo ищет вертикальное видео по запросу и скачивает первое подходящее.
// Возвращает путь к локальному файлу.
func FetchStockVideo(apiKey, query string) (string, error) {
	// Формируем запрос
	u, _ := url.Parse(pexelsBaseURL)
	q := u.Query()
	q.Set("query", query)
	q.Set("per_page", "5")
	q.Set("orientation", "portrait") // только вертикальные
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("создать запрос: %w", err)
	}
	req.Header.Set("Authorization", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("отправить запрос: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API Pexels вернул %d: %s", resp.StatusCode, string(body))
	}

	var pexResp PexelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&pexResp); err != nil {
		return "", fmt.Errorf("декодировать ответ: %w", err)
	}

	if len(pexResp.Videos) == 0 {
		return "", fmt.Errorf("нет видео по запросу: %s", query)
	}

	// Берём первое видео и ищем HD-файл (ширина >= 1080)
	var downloadURL string
	for _, vf := range pexResp.Videos[0].VideoFiles {
		if vf.Width >= 1080 && vf.FileType == "video/mp4" {
			downloadURL = vf.Link
			break
		}
	}
	if downloadURL == "" {
		return "", fmt.Errorf("нет подходящего качества для запроса: %s", query)
	}

	// Скачиваем видео
	videoResp, err := http.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("скачать видео: %w", err)
	}
	defer videoResp.Body.Close()

	// Сохраняем в папку stock/
	os.MkdirAll("stock", 0755)
	filename := filepath.Join("stock", fmt.Sprintf("%s_%d.mp4", sanitizeFilename(query), pexResp.Videos[0].ID))
	file, err := os.Create(filename)
	if err != nil {
		return "", fmt.Errorf("создать файл: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, videoResp.Body)
	if err != nil {
		return "", fmt.Errorf("сохранить видео: %w", err)
	}
	return filename, nil
}

// sanitizeFilename убирает недопустимые символы из имени файла.
func sanitizeFilename(s string) string {
	repl := []string{" ", "/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	res := s
	for _, ch := range repl {
		res = strings.ReplaceAll(res, ch, "_")
	}
	return res
}
