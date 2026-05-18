package collector

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const minimaxMusicAPI = "https://api.minimax.io/v1/music_generation"

type MiniMaxMusicClient struct {
	token  string
	client *http.Client
}

func NewMiniMaxMusicClient(token string) *MiniMaxMusicClient {
	return &MiniMaxMusicClient{
		token:  token,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

type minimaxMusicRequest struct {
	Model          string              `json:"model"`
	Prompt         string              `json:"prompt"`
	IsInstrumental bool                `json:"is_instrumental"`
	AudioSetting   minimaxAudioSetting `json:"audio_setting"`
}

type minimaxAudioSetting struct {
	SampleRate int    `json:"sample_rate"`
	Bitrate    int    `json:"bitrate"`
	Format     string `json:"format"`
}

type minimaxMusicResponse struct {
	Data struct {
		Audio  string `json:"audio"` // hex-encoded
		Status int    `json:"status"`
	} `json:"data"`
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

// GenerateMusic создаёт инструментальную музыку по промпту и длительности (сек).
func (mmc *MiniMaxMusicClient) GenerateMusic(prompt string, duration int) (string, error) {
	if duration < 5 {
		duration = 5
	} else if duration > 30 {
		duration = 30
	}

	reqBody := minimaxMusicRequest{
		Model:          "music-2.6-free",
		Prompt:         prompt,
		IsInstrumental: true,
		AudioSetting: minimaxAudioSetting{
			SampleRate: 44100,
			Bitrate:    256000,
			Format:     "mp3",
		},
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", minimaxMusicAPI, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+mmc.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := mmc.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("MiniMax request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("MiniMax API error %d: %s", resp.StatusCode, string(body))
	}

	var musicResp minimaxMusicResponse
	if err := json.NewDecoder(resp.Body).Decode(&musicResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if musicResp.BaseResp.StatusCode != 0 {
		return "", fmt.Errorf("MiniMax API error: %s (code %d)", musicResp.BaseResp.StatusMsg, musicResp.BaseResp.StatusCode)
	}

	// Декодируем hex в бинарный MP3
	audioBytes, err := hex.DecodeString(musicResp.Data.Audio)
	if err != nil {
		return "", fmt.Errorf("decode hex audio: %w", err)
	}

	os.MkdirAll("backgrounds", 0755)
	filename := filepath.Join("backgrounds", fmt.Sprintf("minimax_music_%d.mp3", time.Now().Unix()))
	if err := os.WriteFile(filename, audioBytes, 0644); err != nil {
		return "", fmt.Errorf("write audio file: %w", err)
	}

	log.Printf("AI-музыка сгенерирована: %s", filename)
	return filename, nil
}
