package generator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// NewGroq создаёт генератор, работающий через Groq API.
func NewGroq(apiKey string) *OpenRouterGenerator {
	return &OpenRouterGenerator{
		APIKey:  apiKey,
		Model:   "llama-3.1-8b-instant", // или "llama-3.3-70b-versatile" для более качественных текстов
		BaseURL: "https://api.groq.com/openai/v1/chat/completions",
	}
}

// OpenRouterGenerator использует бесплатные модели через OpenRouter API
type OpenRouterGenerator struct {
	APIKey  string
	Model   string
	BaseURL string
}

// NewOpenRouter создаёт генератор с бесплатной моделью Google Gemini 2.0 Flash Lite
func NewOpenRouter(apiKey string) *OpenRouterGenerator {
	return &OpenRouterGenerator{
		APIKey:  apiKey,
		Model:   "google/gemini-2.0-flash-lite-001",
		BaseURL: "https://openrouter.ai/api/v1/chat/completions",
	}
}

// GenerateScript генерирует сценарий для видео на русском языке
func (g *OpenRouterGenerator) GenerateScript(title, description string) (string, error) {
	systemPrompt := `Ты — креативный продюсер детского канала (аудитория 7-13 лет).
Создай сценарий для вертикального видео (Shorts) длительностью 30 секунд.
Разбей на 6 коротких предложений для субтитров (каждое ~5 сек).
Пиши весело, позитивно, без жестокости и взрослых тем.

ОЧЕНЬ ВАЖНО: НЕ используй шаблонные фразы вроде "Привет, ребята" или "пишите в комментариях".
Вместо этого ВЫБИРАЙ из списка ниже разные варианты начала и конца.

Варианты приветствия (выбери один или придумай свой в том же стиле):
- Всем салют! 🚀
- Йоу, юные геймеры!
- С вами снова новости из мира игр!
- Привет-привет, друзья!
- Хэй, ребята, готовы к новостям?
- Всем хайп!

Варианты завершения (выбери один или придумай свой, но без слова "комментарии"):
- Делитесь впечатлениями, какая новость круче?
- Что думаете об этом? Пишите своё мнение!
- Обсудим? Жду ваши мысли!
- Ставь лайк, если понравилось!
- Подпишись, чтобы не пропустить новинки!

Язык: строго русский.
Формат ответа: ОДНА строка валидного JSON.
{"full_text":"озвучка целиком","subtitles":["фраза1","фраза2","фраза3","фраза4","фраза5","фраза6"]}`

	userPrompt := fmt.Sprintf("Заголовок: %s\nОписание: %s", title, description)

	reqBody := map[string]interface{}{
		"model": g.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0.8,
		"max_tokens":  300,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", g.BaseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.APIKey)
	req.Header.Set("HTTP-Referer", "https://github.com/your-app")
	req.Header.Set("X-Title", "KidsShortsGenerator")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody bytes.Buffer
		errBody.ReadFrom(resp.Body)
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, errBody.String())
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	content := result.Choices[0].Message.Content

	// Очистка от Markdown-маркеров
	cleaned := removeMarkdownJSON(content)

	start := bytes.IndexByte([]byte(cleaned), '{')
	end := bytes.LastIndexByte([]byte(cleaned), '}')
	if start == -1 || end == -1 || start >= end {
		return "", fmt.Errorf("no JSON found in response: %s", content)
	}

	return cleaned[start : end+1], nil
}

// removeMarkdownJSON убирает ```json и ``` вокруг текста
func removeMarkdownJSON(s string) string {
	s = strings.Replace(s, "```json", "", 1)
	s = strings.Replace(s, "```", "", 1)
	return strings.TrimSpace(s)
}
