package generator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

type OpenRouterGenerator struct {
	APIKey  string
	Model   string
	BaseURL string
	client  *http.Client
}

func NewGroq(apiKey string) *OpenRouterGenerator {
	return &OpenRouterGenerator{
		APIKey:  apiKey,
		Model:   "llama-3.1-8b-instant",
		BaseURL: "https://api.groq.com/openai/v1/chat/completions",
		client:  &http.Client{},
	}
}

func NewOpenRouter(apiKey string) *OpenRouterGenerator {
	return &OpenRouterGenerator{
		APIKey:  apiKey,
		Model:   "google/gemini-2.0-flash-lite-001",
		BaseURL: "https://openrouter.ai/api/v1/chat/completions",
		client:  &http.Client{},
	}
}

func (g *OpenRouterGenerator) GenerateScript(title, description string) (string, error) {
	systemPrompt := `Ты — строгий модератор и креативный продюсер детского канала (аудитория 7-13 лет).
Сначала оцени, подходит ли новость для детей 7-13 лет.
НЕ подходят темы: политика, война, экономика, IT‑конференции, работа, налоги, недвижимость, криминал, взрослые отношения, трагедии, жестокость.
ПОДХОДЯТ темы: игры (Minecraft, Roblox, Brawl Stars и др.), аниме, мемы, блогеры, школьные новости, интересные события, наука для детей, животные, спорт.
ВАЖНО: новости об играх считаются ПОДХОДЯЩИМИ, даже если в них упоминаются слова "ограничения", "блокировка", "Россия", "закон" и т.п. Пример: "Что происходит с метавселенными после ограничения Roblox в России" – это ПОДХОДЯЩАЯ новость, потому что она про игру Roblox.
Если новость НЕ подходит, верни СТРОГО {"skip":true} и больше ничего.
Если новость ПОДХОДИТ, создай сценарий для вертикального видео (Shorts) длительностью 30 секунд.
Разбей на 6 коротких предложений для субтитров (каждое ~5 сек).
Пиши весело, позитивно, без жестокости и взрослых тем.
ВАЖНО: пиши строго на русском языке, не используй другие языки.

ЗАПРЕЩЕНО использовать одни и те же приветствия и прощания. Каждый раз выбирай из списка ниже новый вариант, не повторяйся.

Варианты начала (выбери один, заменяй похожие слова, или придумай аналогичное):
- "Привет, друзья! 👋"
- "Смотри, что нашли!"
- "А вы знали, что..."
- "Готовы к новости дня?"
- "Срочно в номер!"
- "Это просто бомба! 💣"
- "Хей-хей, геймеры!"
- "Ого, вы только гляньте!"
- "Всем привет, искатели приключений!"
- "Здарова, народ!"
- "Кто готов к движу?"

Варианты концовки (выбери один, заменяй похожие слова, или придумай аналогичное):
- "Обсудим в комментах?"
- "А как бы сделали вы?"
- "Жду ваши мысли!"
- "Что думаете? Пиши!"
- "Делитесь мнением 👇"
- "Как вам такая идея?"
- "Напишите, что выберете!"
- "Ваш черёд!"

Верни строго JSON без markdown-разметки:
Если новость не подходит: {"skip":true}
Если подходит: {"full_text":"озвучка целиком","subtitles":["фраза1","фраза2","фраза3","фраза4","фраза5","фраза6"]}
Озвучка должна начинаться с приветствия и заканчиваться призывом к обсуждению.`

	// Первая попытка
	content, err := g.callAPI(systemPrompt, fmt.Sprintf("Заголовок: %s\nОписание: %s", title, description))
	if err != nil {
		return "", err
	}

	jsonStr, err := extractJSON(content)
	if err == nil {
		return jsonStr, nil
	}

	// JSON не найден — пробуем строгий промпт
	log.Printf("Первая попытка не дала JSON, пробую снова со строгим промптом")
	strictSystem := "Return ONLY a valid JSON object as specified. No other text, no markdown. All content in Russian."
	content, err = g.callAPI(strictSystem, fmt.Sprintf("Заголовок: %s\nОписание: %s", title, description))
	if err != nil {
		return "", err
	}

	jsonStr, err = extractJSON(content)
	if err != nil {
		return "", fmt.Errorf("no JSON found in response even after strict prompt: %s", content)
	}
	return jsonStr, nil
}

// callAPI отправляет запрос и возвращает содержимое ответа.
func (g *OpenRouterGenerator) callAPI(system, user string) (string, error) {
	reqBody := map[string]interface{}{
		"model": g.Model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"temperature": 0.9,
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

	resp, err := g.client.Do(req)
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
	return result.Choices[0].Message.Content, nil
}

// extractJSON извлекает JSON из текста, убирая маркеры и пробелы.
func extractJSON(raw string) (string, error) {
	// Убираем ```json и ``` (с любыми отступами)
	cleaned := strings.ReplaceAll(raw, "```json", "")
	cleaned = strings.ReplaceAll(cleaned, "```", "")
	cleaned = strings.TrimSpace(cleaned)

	start := strings.Index(cleaned, "{")
	end := strings.LastIndex(cleaned, "}")
	if start == -1 || end == -1 || start >= end {
		return "", fmt.Errorf("no JSON braces found")
	}
	return cleaned[start : end+1], nil
}

// ExtractKeywords возвращает 7-10 английских ключевых слов по заголовку и описанию новости.
func (g *OpenRouterGenerator) ExtractKeywords(title, description string) ([]string, error) {
	systemPrompt := `You are a helpful assistant that extracts keywords from Russian news headlines and descriptions for a kids' channel (ages 7-13). 
The keywords will be used to search for background videos and images.
Extract 5-7 very specific and relevant English keywords, prioritizing rare or unique words from the text (like proper names, game titles, specific events). 
Avoid generic terms like "social media", "internet", "popular", "online", "trending" unless they are the only relevant words.
Return ONLY a JSON array of strings, without markdown.`

	userPrompt := fmt.Sprintf("Title: %s\nDescription: %s", title, description)

	reqBody := map[string]interface{}{
		"model": g.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0.5,
		"max_tokens":  100,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", g.BaseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.APIKey)
	req.Header.Set("HTTP-Referer", "https://github.com/your-app")
	req.Header.Set("X-Title", "KidsShortsGenerator")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody bytes.Buffer
		errBody.ReadFrom(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, errBody.String())
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	content := result.Choices[0].Message.Content
	cleaned := strings.ReplaceAll(strings.ReplaceAll(content, "```json", ""), "```", "")
	cleaned = strings.TrimSpace(cleaned)

	start := strings.IndexByte(cleaned, '[')
	end := strings.LastIndexByte(cleaned, ']')
	if start == -1 || end == -1 || start >= end {
		return nil, fmt.Errorf("no JSON array found in response: %s", content)
	}

	var keywords []string
	if err := json.Unmarshal([]byte(cleaned[start:end+1]), &keywords); err != nil {
		return nil, fmt.Errorf("unmarshal keywords: %w", err)
	}
	return keywords, nil
}
