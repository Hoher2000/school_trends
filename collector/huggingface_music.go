package collector

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const musicGenSpace = "https://facebook-musicgen.hf.space/api/predict"

type HuggingFaceMusicClient struct {
	client *http.Client
}

func NewHuggingFaceMusicClient() *HuggingFaceMusicClient {
	return &HuggingFaceMusicClient{
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

// GenerateMusic генерирует музыку через Gradio-демо MusicGen.
func (hfm *HuggingFaceMusicClient) GenerateMusic(prompt string, duration int) (string, error) {
	if duration < 5 {
		duration = 5
	} else if duration > 30 {
		duration = 30
	}

	// Аргументы для Gradio: текст, длительность, стратегия (top-p, temperature и т.д.)
	reqData := map[string]interface{}{
		"data": []interface{}{
			prompt,            // описание
			float64(duration), // длительность в секундах
			nil,               // temperature (необязательно)
		},
	}
	jsonData, _ := json.Marshal(reqData)

	req, _ := http.NewRequest("POST", musicGenSpace, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	resp, err := hfm.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Gradio запрос: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Gradio ошибка %d: %s", resp.StatusCode, string(body))
	}

	// Ответ Gradio содержит массив data, где второй элемент – аудио как Base64 или путь
	var result struct {
		Data []interface{} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("парсинг ответа Gradio: %w", err)
	}

	if len(result.Data) < 2 {
		return "", fmt.Errorf("недостаточно данных в ответе")
	}

	// Второй элемент – аудио в виде base64-строки
	audioBase64, ok := result.Data[1].(string)
	if !ok {
		return "", fmt.Errorf("не удалось извлечь аудио из ответа")
	}

	// Декодируем и сохраняем
	audioBytes, err := base64.StdEncoding.DecodeString(audioBase64)
	if err != nil {
		return "", fmt.Errorf("декодирование base64: %w", err)
	}

	os.MkdirAll("backgrounds", 0755)
	filename := filepath.Join("backgrounds", fmt.Sprintf("hf_music_%d.mp3", time.Now().Unix()))
	if err := os.WriteFile(filename, audioBytes, 0644); err != nil {
		return "", fmt.Errorf("запись файла: %w", err)
	}

	fmt.Printf("AI-музыка сгенерирована: %s\n", filename)
	return filename, nil
}
