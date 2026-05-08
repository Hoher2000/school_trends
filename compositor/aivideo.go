package compositor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const replicateBaseURL = "https://api.replicate.com/v1"

type modelResponse struct {
	LatestVersion struct {
		ID string `json:"id"`
	} `json:"latest_version"`
}

type replicateRequest struct {
	Version string `json:"version,omitempty"`
	Input   struct {
		Prompt      string `json:"prompt"`
		Duration    int    `json:"duration"`
		AspectRatio string `json:"aspect_ratio"`
	} `json:"input"`
}

type replicateResponse struct {
	ID     string      `json:"id"`
	Status string      `json:"status"`
	Output interface{} `json:"output"` // может быть строкой (URL) или объектом
	Error  string      `json:"error"`
}

// getLatestVersion получает последнюю версию модели minimax/video-01
func getLatestVersion(apiKey, modelName string) (string, error) {
	req, err := http.NewRequest("GET", replicateBaseURL+"/models/"+modelName, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Token "+apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch model info: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("model info error %d: %s", resp.StatusCode, string(body))
	}
	var m modelResponse
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return "", fmt.Errorf("decode model info: %w", err)
	}
	if m.LatestVersion.ID == "" {
		return "", fmt.Errorf("empty latest version for model %s", modelName)
	}
	return m.LatestVersion.ID, nil
}

func GenerateAIVideo(apiKey, prompt string) (string, error) {
	version, err := getLatestVersion(apiKey, "minimax/video-01")
	if err != nil {
		return "", fmt.Errorf("get version: %w", err)
	}

	reqBody := replicateRequest{}
	reqBody.Version = version
	reqBody.Input.Prompt = prompt + ", vertical video for kids, fun, colorful, engaging"
	reqBody.Input.Duration = 5
	reqBody.Input.AspectRatio = "9:16"

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", replicateBaseURL+"/predictions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Token "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Replicate API error %d: %s", resp.StatusCode, string(body))
	}

	var prediction replicateResponse
	if err := json.NewDecoder(resp.Body).Decode(&prediction); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	// Ожидаем завершения генерации (опрашиваем статус)
	for prediction.Status != "succeeded" && prediction.Status != "failed" {
		time.Sleep(5 * time.Second)
		statusReq, _ := http.NewRequest("GET", replicateBaseURL+"/predictions/"+prediction.ID, nil)
		statusReq.Header.Set("Authorization", "Token "+apiKey)
		statusResp, err := client.Do(statusReq)
		if err != nil {
			return "", fmt.Errorf("status request: %w", err)
		}
		if err := json.NewDecoder(statusResp.Body).Decode(&prediction); err != nil {
			statusResp.Body.Close()
			return "", fmt.Errorf("decode status: %w", err)
		}
		statusResp.Body.Close()
	}

	if prediction.Status == "failed" {
		return "", fmt.Errorf("AI generation failed: %s", prediction.Error)
	}

	// Извлекаем URL видео из output
	videoURL, err := extractVideoURL(prediction.Output)
	if err != nil {
		return "", fmt.Errorf("extract video URL: %w", err)
	}

	// Скачиваем видео
	videoResp, err := http.Get(videoURL)
	if err != nil {
		return "", fmt.Errorf("download video: %w", err)
	}
	defer videoResp.Body.Close()

	os.MkdirAll("stock", 0755)
	filename := filepath.Join("stock", fmt.Sprintf("ai_gen_%s.mp4", time.Now().Format("150405")))
	file, err := os.Create(filename)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, videoResp.Body)
	if err != nil {
		return "", fmt.Errorf("save video: %w", err)
	}
	return filename, nil
}

// extractVideoURL извлекает URL видео из поля output (строка или map[string]interface{})
func extractVideoURL(output any) (string, error) {
	if output == nil {
		return "", fmt.Errorf("output is nil")
	}
	switch v := output.(type) {
	case string:
		return v, nil
	case map[string]interface{}:
		if url, ok := v["url"].(string); ok {
			return url, nil
		}
		if url, ok := v["video"].(string); ok {
			return url, nil
		}
		return "", fmt.Errorf("no 'url' or 'video' in output map")
	default:
		return "", fmt.Errorf("unexpected output type %T", output)
	}
}
