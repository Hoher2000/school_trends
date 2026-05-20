package publisher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

const vkAPI = "https://api.vk.com/method/"

type VKVideoUploadResponse struct {
	Response struct {
		UploadURL string `json:"upload_url"`
		VideoID   int    `json:"video_id"`
		OwnerID   int    `json:"owner_id"`
	} `json:"response"`
}

type VKWallPostResponse struct {
	Response struct {
		PostID int `json:"post_id"`
	} `json:"response"`
}

// PostVideo публикует видео и текст в сообщество ВКонтакте.
func PostVideo(accessToken, groupID, videoPath, caption string) error {
	groupIDInt, err := strconv.Atoi(groupID)
	if err != nil {
		return fmt.Errorf("некорректный group ID: %w", err)
	}

	// 1. Получаем ссылку для загрузки видео
	uploadURL, videoID, ownerID, err := getVideoUploadURL(accessToken, groupIDInt)
	if err != nil {
		return fmt.Errorf("получить upload URL: %w", err)
	}

	// 2. Загружаем видеофайл
	err = uploadFile(uploadURL, videoPath)
	if err != nil {
		return fmt.Errorf("загрузить видео: %w", err)
	}

	// 3. Публикуем пост с видео на стене сообщества
	err = postToWall(accessToken, groupIDInt, videoID, ownerID, caption)
	if err != nil {
		return fmt.Errorf("опубликовать пост: %w", err)
	}
	return nil
}

func getVideoUploadURL(accessToken string, groupID int) (string, int, int, error) {
	url := fmt.Sprintf("%svideo.save?access_token=%s&group_id=-%d&v=5.199", vkAPI, accessToken, groupID)
	req, err := http.NewRequest("POST", url, nil) // ← POST вместо GET
	if err != nil {
		return "", 0, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, 0, err
	}
	defer resp.Body.Close()

	var uploadResp VKVideoUploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&uploadResp); err != nil {
		return "", 0, 0, err
	}
	if uploadResp.Response.UploadURL == "" {
		// выводим сырой ответ для отладки
		body, _ := io.ReadAll(resp.Body)
		return "", 0, 0, fmt.Errorf("пустой upload URL. Ответ VK: %s", string(body))
	}
	return uploadResp.Response.UploadURL, uploadResp.Response.VideoID, uploadResp.Response.OwnerID, nil
}

func uploadFile(uploadURL, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("video_file", filepath.Base(filePath))
	io.Copy(part, file)
	writer.Close()

	req, _ := http.NewRequest("POST", uploadURL, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed: %s", respBody)
	}
	return nil
}

func postToWall(accessToken string, groupID, videoID, ownerID int, message string) error {
	attachment := fmt.Sprintf("video%d_%d", ownerID, videoID)
	url := fmt.Sprintf("%swall.post?access_token=%s&owner_id=-%d&from_group=1&message=%s&attachments=%s&v=5.199",
		vkAPI, accessToken, groupID, message, attachment)

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var wallResp VKWallPostResponse
	if err := json.NewDecoder(resp.Body).Decode(&wallResp); err != nil {
		return err
	}
	if wallResp.Response.PostID == 0 {
		return fmt.Errorf("wall.post вернул 0")
	}
	return nil
}
