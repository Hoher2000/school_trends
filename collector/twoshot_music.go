package collector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const twoShotBaseURL = "https://api.twoshot.ai/v1"

type TwoShotMusicClient struct {
	client *http.Client
}

func NewTwoShotMusicClient() *TwoShotMusicClient {
	return &TwoShotMusicClient{
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

type twoShotGenRequest struct {
	Prompt   string `json:"prompt"`
	Duration int    `json:"duration"` // в секундах
}

type twoShotGenResponse struct {
	GenerationID string `json:"generation_id"`
}

type twoShotStatusResponse struct {
	Status    string `json:"status"`
	OutputURL string `json:"output_url,omitempty"`
	Error     string `json:"error,omitempty"`
}

// GenerateMusic создаёт музыку по текстовому описанию и длительности (сек).
// duration автоматически ограничивается диапазоном 5–30 сек.
func (tsm *TwoShotMusicClient) GenerateMusic(prompt string, duration int) (string, error) {
	if duration < 5 {
		duration = 5
	} else if duration > 30 {
		duration = 30
	}

	// 1. Запуск генерации
	reqBody := twoShotGenRequest{Prompt: prompt, Duration: duration}
	jsonData, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", twoShotBaseURL+"/generate", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "KidsShortsGenerator/1.0")

	resp, err := tsm.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ошибка запроса генерации: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API вернул %d: %s", resp.StatusCode, string(body))
	}

	var genResp twoShotGenResponse
	if err := json.NewDecoder(resp.Body).Decode(&genResp); err != nil {
		return "", fmt.Errorf("парсинг ответа: %w", err)
	}

	// 2. Ожидание завершения
	for {
		time.Sleep(3 * time.Second)
		statusReq, _ := http.NewRequest("GET", fmt.Sprintf("%s/generations/%s", twoShotBaseURL, genResp.GenerationID), nil)
		statusReq.Header.Set("User-Agent", "KidsShortsGenerator/1.0")

		statusResp, err := tsm.client.Do(statusReq)
		if err != nil {
			return "", fmt.Errorf("проверка статуса: %w", err)
		}

		var status twoShotStatusResponse
		if err := json.NewDecoder(statusResp.Body).Decode(&status); err != nil {
			statusResp.Body.Close()
			return "", fmt.Errorf("парсинг статуса: %w", err)
		}
		statusResp.Body.Close()

		if status.Status == "completed" {
			return tsm.downloadAudio(status.OutputURL)
		} else if status.Status == "failed" {
			return "", fmt.Errorf("генерация не удалась: %s", status.Error)
		}
		log.Printf("Жду генерацию музыки: %s", status.Status)
	}
}

func (tsm *TwoShotMusicClient) downloadAudio(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("скачивание аудио: %w", err)
	}
	defer resp.Body.Close()

	os.MkdirAll("backgrounds", 0755)
	filename := filepath.Join("backgrounds", fmt.Sprintf("ai_music_%d.mp3", time.Now().Unix()))
	file, err := os.Create(filename)
	if err != nil {
		return "", fmt.Errorf("создание файла: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return "", fmt.Errorf("сохранение аудио: %w", err)
	}
	return filename, nil
}
