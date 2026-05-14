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
	"time"
)

const (
	pexelsBaseURL  = "https://api.pexels.com/videos/search"
	pixabayBaseURL = "https://pixabay.com/api/videos/"
)

type PexelsVideo struct {
	ID          int    `json:"id"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	VideoFiles  []struct {
		Link     string `json:"link"`
		Width    int    `json:"width"`
		Height   int    `json:"height"`
		FileType string `json:"file_type"`
	} `json:"video_files"`
}

type PexelsResponse struct {
	Videos []PexelsVideo `json:"videos"`
}

type PixabayVideo struct {
	ID     int `json:"id"`
	Videos struct {
		Large struct {
			URL    string `json:"url"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"large"`
	} `json:"videos"`
}

type PixabayResponse struct {
	Hits []PixabayVideo `json:"hits"`
}

var kidUnsafeKeywords = []string{
	// насилие / взрослое
	"shooting", "gun", "violence", "war", "military", "weapon",
	"adult", "sexy", "nude", "alcohol", "smoking", "drug",
	"horror", "blood", "death", "kill", "fight",
	// дошкольники / детские центры
	"preschool", "toddler", "baby", "daycare", "nursery",
	"kindergarten", "childcare", "playgroup", "creche",
}

func isKidSafeVideo(video PexelsVideo) bool {
	text := strings.ToLower(video.Title + " " + video.Description)
	for _, kw := range kidUnsafeKeywords {
		if strings.Contains(text, kw) {
			return false
		}
	}
	return true
}

// ExtractKeywords извлекает ключевые слова из заголовка статьи.
func ExtractKeywords(title string) string {
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

func FetchStockVideo(apiKey, query string, client *http.Client) (string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if query == "" {
		query = "kids fun gameplay"
	}

	// Основной запрос
	path, err := searchPexels(apiKey, query, client)
	if err == nil {
		return path, nil
	}
	log.Printf("Pexels (основной запрос) не дал результатов: %v", err)

	// Тематические фолбэки
	fallbackQueries := []string{
		"minecraft gameplay " + fmt.Sprintf("%d", time.Now().UnixNano()%100),
		"anime fight scene " + fmt.Sprintf("%d", time.Now().UnixNano()%100),
		"roblox gameplay " + fmt.Sprintf("%d", time.Now().UnixNano()%100),
		"brawl stars gameplay " + fmt.Sprintf("%d", time.Now().UnixNano()%100),
		"epic gaming moments " + fmt.Sprintf("%d", time.Now().UnixNano()%100),
	}
	for _, fq := range fallbackQueries {
		path, err = searchPexels(apiKey, fq, client)
		if err == nil {
			log.Printf("Pexels (фолбэк '%s') вернул результат", fq)
			return path, nil
		}
		log.Printf("Pexels (фолбэк '%s') не дал результатов: %v", fq, err)
	}

	if pixabayKey := os.Getenv("PIXABAY_API_KEY"); pixabayKey != "" {
		path, err = searchPixabay(pixabayKey, query, client)
		if err == nil {
			return path, nil
		}
		log.Printf("Pixabay не дал результатов: %v", err)
	}

	return "", fmt.Errorf("ни один источник не вернул подходящее видео")
}

func searchPexels(apiKey, query string, client *http.Client) (string, error) {
	u, _ := url.Parse(pexelsBaseURL)
	q := u.Query()
	q.Set("query", query)
	q.Set("per_page", "15")
	q.Set("orientation", "portrait")
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("создать запрос: %w", err)
	}
	req.Header.Set("Authorization", apiKey)

	resp, err := client.Do(req)
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

	for _, video := range pexResp.Videos {
		if !isKidSafeVideo(video) {
			continue
		}
		for _, vf := range video.VideoFiles {
			if vf.Width >= 1080 && vf.FileType == "video/mp4" {
				return downloadVideo(vf.Link, client, "pexels_"+sanitizeFilename(query))
			}
		}
	}
	return "", fmt.Errorf("нет подходящего видео")
}

func searchPixabay(apiKey, query string, client *http.Client) (string, error) {
	u, _ := url.Parse(pixabayBaseURL)
	q := u.Query()
	q.Set("key", apiKey)
	q.Set("q", query)
	q.Set("per_page", "15")
	q.Set("safesearch", "true")
	u.RawQuery = q.Encode()

	resp, err := client.Get(u.String())
	if err != nil {
		return "", fmt.Errorf("Pixabay запрос: %w", err)
	}
	defer resp.Body.Close()

	var pixResp PixabayResponse
	if err := json.NewDecoder(resp.Body).Decode(&pixResp); err != nil {
		return "", fmt.Errorf("Pixabay decode: %w", err)
	}

	for _, video := range pixResp.Hits {
		if video.Videos.Large.URL != "" && video.Videos.Large.Width >= 1080 {
			return downloadVideo(video.Videos.Large.URL, client, "pixabay_"+sanitizeFilename(query))
		}
	}
	return "", fmt.Errorf("Pixabay: нет видео")
}

func downloadVideo(url string, client *http.Client, prefix string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("создать запрос на скачивание: %w", err)
	}
	videoResp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("скачать видео: %w", err)
	}
	defer videoResp.Body.Close()

	os.MkdirAll("stock", 0755)
	filename := filepath.Join("stock", fmt.Sprintf("%s_%d.mp4", prefix, time.Now().UnixNano()))
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
