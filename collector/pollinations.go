// collector/pollinations.go
package collector

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func FetchPollinationsImage(prompt string) (string, error) {
	baseURL := "https://image.pollinations.ai/prompt/"
	params := url.Values{}
	params.Set("width", "1080")
	params.Set("height", "1920")
	params.Set("model", "flux")
	fullURL := fmt.Sprintf("%s%s?%s", baseURL, url.QueryEscape(prompt), params.Encode())

	resp, err := http.Get(fullURL)
	if err != nil {
		return "", fmt.Errorf("pollinations request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("pollinations API error %d: %s", resp.StatusCode, string(body))
	}

	os.MkdirAll("backgrounds", 0755)
	filename := filepath.Join("backgrounds", fmt.Sprintf("pollinations_%d.jpg", time.Now().Unix()))
	file, err := os.Create(filename)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return "", fmt.Errorf("save image: %w", err)
	}
	return filename, nil
}

// FetchPollinationsImageWithRetry пытается скачать изображение несколько раз.
func FetchPollinationsImageWithRetry(prompt string, maxRetries int) (string, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		imgPath, err := FetchPollinationsImage(prompt)
		if err == nil {
			return imgPath, nil
		}
		lastErr = err
		if strings.Contains(err.Error(), "402") {
			// Очередь полна – ждём 5 секунд и пробуем снова
			time.Sleep(5 * time.Second)
		} else {
			// Не 402 – дальше пытаться бессмысленно
			break
		}
	}
	return "", fmt.Errorf("не удалось после %d попыток: %w", maxRetries, lastErr)
}
