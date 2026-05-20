package publisher

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

// SendVideoToTelegram отправляет видео и подпись в Telegram через Bot API.
func SendVideoToTelegram(token, chatID, videoPath, caption string) error {
	// Открываем файл
	file, err := os.Open(videoPath)
	if err != nil {
		return fmt.Errorf("открыть видео: %w", err)
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Поле chat_id
	writer.WriteField("chat_id", chatID)
	// Поле caption
	writer.WriteField("caption", caption)

	// Поле video
	part, err := writer.CreateFormFile("video", filepath.Base(videoPath))
	if err != nil {
		return fmt.Errorf("создать форму: %w", err)
	}
	_, err = io.Copy(part, file)
	if err != nil {
		return fmt.Errorf("копировать видео: %w", err)
	}
	writer.Close()

	// Отправка запроса
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendVideo", token)
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return fmt.Errorf("создать запрос: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("отправить запрос: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ошибка Telegram API %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
