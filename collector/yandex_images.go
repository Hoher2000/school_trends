package collector

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type YandexImageResult struct {
	Title string `json:"title"`
	Image struct {
		URL    string `json:"url"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	} `json:"image"`
}

type yandexResponse struct {
	Results []YandexImageResult `json:"results"`
}

func FetchYandexImages(query string, limit int) ([]string, error) {
	baseURL := "http://127.0.0.1:7000/yandex/image"
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	q := u.Query()
	q.Set("text", query)
	q.Set("limit", "30")
	q.Set("family", "2")
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("openserp request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openserp returned %d", resp.StatusCode)
	}

	var yr yandexResponse
	if err := json.NewDecoder(resp.Body).Decode(&yr); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var urls []string
	for _, r := range yr.Results {
		if r.Image.URL != "" {
			urls = append(urls, r.Image.URL)
		}
	}
	return urls, nil
}
