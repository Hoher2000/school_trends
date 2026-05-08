package compositor

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
)

const pexelsBaseURL = "https://api.pexels.com/videos/search"

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

type PexelsResponse struct {
	Videos []PexelsVideo `json:"videos"`
}

func FetchStockVideo(apiKey, rawTitle string) (string, error) {
	query := extractKeywords(rawTitle)
	if query == "" {
		query = "kids fun"
	}

	path, err := searchAndDownload(apiKey, query)
	if err == nil {
		return path, nil
	}
	log.Printf("Первичный запрос '%s' не дал результатов: %v", query, err)

	fallback := "kids fun"
	path, err = searchAndDownload(apiKey, fallback)
	if err != nil {
		return "", fmt.Errorf("ни основной, ни запасной запрос не вернули видео: %w", err)
	}
	return path, nil
}

func extractKeywords(title string) string {
	words := strings.Fields(title)
	if len(words) > 4 {
		words = words[:4]
	}
	result := strings.Join(words, " ")
	var reg strings.Builder
	for _, r := range result {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ' {
			reg.WriteRune(r)
		}
	}
	return strings.TrimSpace(reg.String())
}

func searchAndDownload(apiKey, query string) (string, error) {
	u, _ := url.Parse(pexelsBaseURL)
	q := u.Query()
	q.Set("query", query)
	q.Set("per_page", "5")
	q.Set("orientation", "portrait")
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

	videoResp, err := http.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("скачать видео: %w", err)
	}
	defer videoResp.Body.Close()

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

func sanitizeFilename(s string) string {
	repl := []string{" ", "/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	res := s
	for _, ch := range repl {
		res = strings.ReplaceAll(res, ch, "_")
	}
	return res
}
