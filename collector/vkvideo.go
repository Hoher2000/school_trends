package collector

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

type VKVideoResponse struct {
	Response struct {
		Items []struct {
			ID          int    `json:"id"`
			OwnerID     int    `json:"owner_id"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Player      string `json:"player"`
		} `json:"items"`
	} `json:"response"`
}

// FetchVKVideo ищет видео в VK и сохраняет ссылку на плеер.
func FetchVKVideo(accessToken, query string) (string, error) {
	u, _ := url.Parse("https://api.vk.com/method/video.search")
	q := u.Query()
	q.Set("q", query)
	q.Set("access_token", accessToken)
	q.Set("v", "5.199")
	q.Set("count", "5")
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		return "", fmt.Errorf("VK API: %w", err)
	}
	defer resp.Body.Close()

	var vkResp VKVideoResponse
	if err := json.NewDecoder(resp.Body).Decode(&vkResp); err != nil {
		return "", fmt.Errorf("VK decode: %w", err)
	}
	if len(vkResp.Response.Items) == 0 {
		return "", fmt.Errorf("VK: нет видео")
	}

	video := vkResp.Response.Items[0]
	os.MkdirAll("backgrounds", 0755)
	filename := filepath.Join("backgrounds", fmt.Sprintf("vk_%s_%d_%d.txt", sanitizeFilename(video.Title), video.OwnerID, video.ID))
	f, err := os.Create(filename)
	if err != nil {
		return "", fmt.Errorf("создать файл: %w", err)
	}
	defer f.Close()
	fmt.Fprintf(f, "%s\n%s\n", video.Player, video.Title)
	return filename, nil
}
