package tts

import (
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	easyjson "github.com/mailru/easyjson"
)

// TokenResponse – ответ OAuth-сервера
//
//easyjson:json
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

// TokenManager автоматически управляет токеном доступа SaluteSpeech
type TokenManager struct {
	clientID     string
	clientSecret string
	scope        string
	tokenURL     string

	mu           sync.RWMutex
	currentToken string
	refreshToken string
	expiresAt    time.Time
	stopCh       chan struct{}
	doneCh       chan struct{}
	httpClient   *http.Client
	stopOnce     sync.Once
}

//easyjson:json
type SavedToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// NewTokenManager создаёт менеджер токенов
func NewTokenManager(clientID, clientSecret, scope, tokenURL string) *TokenManager {
	// HTTP‑клиент, игнорирующий сертификаты Минцифры
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	tm := &TokenManager{
		clientID:     clientID,
		clientSecret: clientSecret,
		scope:        scope,
		tokenURL:     tokenURL,
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
		httpClient:   client,
	}
	// пытаемся загрузить токен из локального кэша
	tm.loadFromFile("token_cache.json")
	return tm
}

// Start получает первый токен и запускает фоновое обновление
func (tm *TokenManager) Start() error {
	if err := tm.refreshTokenNow(); err != nil {
		return fmt.Errorf("ошибка первичного получения токена: %w", err)
	}
	go tm.refreshLoop()
	log.Println("[TokenManager] Запущено автоматическое обновление токена")
	return nil
}

// GetToken возвращает текущий access token (потокобезопасно)
func (tm *TokenManager) GetToken() string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.currentToken
}

// Stop останавливает фоновое обновление
func (tm *TokenManager) Stop() {
	tm.stopOnce.Do(func() {
		close(tm.stopCh)
		<-tm.doneCh
		log.Println("[TokenManager] Остановлено")
	})
}

// refreshLoop проверяет необходимость обновления каждые 20 минут
func (tm *TokenManager) refreshLoop() {
	defer close(tm.doneCh)
	ticker := time.NewTicker(20 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tm.mu.RLock()
			expiresIn := time.Until(tm.expiresAt)
			tm.mu.RUnlock()
			if expiresIn < 5*time.Minute {
				log.Printf("[TokenManager] Токен истекает через %v, обновляю", expiresIn)
				if err := tm.refreshTokenNow(); err != nil {
					log.Printf("[TokenManager] Ошибка обновления: %v", err)
				}
			}
		case <-tm.stopCh:
			return
		}
	}
}

// refreshTokenNow выполняет запрос к серверу авторизации
func (tm *TokenManager) refreshTokenNow() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	maxRetries := 3
	baseDelay := time.Second

	for attempt := range maxRetries {
		if attempt > 0 {
			delay := baseDelay * time.Duration(1<<(attempt-1)) // 1s, 2s, 4s
			log.Printf("[TokenManager] Повторная попытка %d/%d через %v", attempt+1, maxRetries, delay)
			time.Sleep(delay)
		}

		var formData url.Values
		if tm.refreshToken != "" {
			formData = url.Values{
				"grant_type":    {"refresh_token"},
				"refresh_token": {tm.refreshToken},
				"scope":         {tm.scope},
			}
		} else {
			formData = url.Values{
				"grant_type": {"client_credentials"},
				"scope":      {tm.scope},
			}
		}

		body := strings.NewReader(formData.Encode())
		req, err := http.NewRequest("POST", tm.tokenURL, body)
		if err != nil {
			return fmt.Errorf("ошибка создания запроса: %w", err)
		}

		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("RqUID", generateRqUID())
		req.SetBasicAuth(tm.clientID, tm.clientSecret)

		resp, err := tm.httpClient.Do(req)
		if err != nil {
			log.Printf("[TokenManager] Попытка %d провалена: %v", attempt+1, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			log.Printf("[TokenManager] Попытка %d: сервер вернул %d: %s", attempt+1, resp.StatusCode, string(bodyBytes))
			continue
		}

		tokenResp := TokenResponse{}
		if err := easyjson.UnmarshalFromReader(resp.Body, &tokenResp); err != nil {
			return fmt.Errorf("ошибка парсинга ответа: %w", err)
		}

		tm.currentToken = tokenResp.AccessToken
		if tokenResp.RefreshToken != "" {
			tm.refreshToken = tokenResp.RefreshToken
		}
		tm.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
		tm.saveToFile("token_cache.json")
		log.Printf("[TokenManager] Токен получен, действителен до %v", tm.expiresAt)
		return nil
	}

	return fmt.Errorf("не удалось получить токен после %d попыток", maxRetries)
}

// генерация уникального RqUID
func generateRqUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// вспомогательные методы для кэширования токена в файл
func (tm *TokenManager) saveToFile(path string) {
	data, _ := easyjson.Marshal(&SavedToken{
		AccessToken:  tm.currentToken,
		RefreshToken: tm.refreshToken,
		ExpiresAt:    tm.expiresAt,
	})
	os.WriteFile(path, data, 0600)
}

func (tm *TokenManager) loadFromFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	saved := &SavedToken{}
	if err := easyjson.Unmarshal(data, saved); err != nil {
		return
	}
	tm.currentToken = saved.AccessToken
	tm.refreshToken = saved.RefreshToken
	tm.expiresAt = saved.ExpiresAt
}
