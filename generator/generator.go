package generator

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//easyjson:json
type Script struct {
	FullText  string   `json:"full_text"`
	Subtitles []string `json:"subtitles"`
	Skip      bool     `json:"skip,omitempty"`
}

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
Весь JSON должен быть в одну строку, без переносов. Если внутри full_text есть кавычки, экранируй их как \". Переносы строк замени на \n. Ответ должен быть СТРОГО валидным JSON объектом.
НЕ подходят темы: политика, война, экономика, IT‑конференции, работа, налоги, недвижимость, криминал, взрослые отношения, трагедии, жестокость.

ПОДХОДЯТ темы: игры (Minecraft, Roblox, Brawl Stars и др.), аниме, мемы, блогеры, школьные новости, интересные события, наука для детей, животные, спорт, TikTok-тренды, челленджи, вирусные видео, гаджеты, технологии.

ВАЖНО – ЭТО ЖЁСТКОЕ ПРАВИЛО: Если заголовок содержит слова "аниме", "манга", "Minecraft", "Roblox", "Brawl Stars", "TikTok", "челлендж", "гаджет", "игра", "школьник", "мем" — **ТЫ ОБЯЗАН** создать сценарий. skip:true для таких новостей **ЗАПРЕЩЁН**. Никаких исключений. Даже если тебе кажется, что новость не подходит, всё равно сгенерируй сценарий, потому что мы вручную подтверждаем безопасность таких тем.

Примеры ПОДХОДЯЩИХ новостей (все они должны получить сценарий, а не skip):
- "Новое обновление в Minecraft - «Деревня Гномов»"
- "Лучший гаджет для школьника - «Сумка Планшет»"
- "Челлендж «Танцуй, как хаш-ла-бура» на TikTok"
- "Новый мем с лисой захватил соцсети"

Примеры НЕподходящих новостей (их можно пропускать):
- "Изменения в налоговом кодексе РФ"
- "Конференция по искусственному интеллекту для бизнеса"

Если новость НЕ подходит, верни СТРОГО {"skip":true} и больше ничего.
Если новость ПОДХОДИТ, создай сценарий для вертикального видео (Shorts) длительностью 30 секунд.
Разбей на 6 коротких предложений для субтитров (каждое ~5 сек).
Пиши весело, позитивно, без жестокости и взрослых тем.
ВАЖНО: пиши строго на русском языке, не используй другие языки.

ЗАПРЕЩЕНО использовать одни и те же приветствия и прощания. Каждый раз выбирай из списка ниже новый вариант, не повторяйся.

Варианты начала (заменяй похожие слова, или придумай аналогичное):
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

Варианты концовки (заменяй похожие слова, или придумай аналогичное):
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
	content, err := g.callAPI(systemPrompt, fmt.Sprintf("Заголовок: %s\nОписание: %s", title, description), 800)
	if err != nil {
		return "", err
	}

	jsonStr, err := extractJSON(content)
	if err == nil {
		jsonStr = strings.TrimSpace(jsonStr)
		// Убедимся, что начинается с '{' и заканчивается '}'
		if strings.HasPrefix(jsonStr, "{") && strings.HasSuffix(jsonStr, "}") {
			return jsonStr, nil
		}
		// Если extractJSON вернул что-то не то, обрежем до первого объекта
		if idx := strings.Index(jsonStr, "{"); idx != -1 {
			jsonStr = jsonStr[idx:]
			if end := strings.LastIndex(jsonStr, "}"); end != -1 {
				jsonStr = jsonStr[:end+1]
			}
			return jsonStr, nil
		}
	}

	// JSON не найден — пробуем строгий промпт
	log.Printf("Первая попытка не дала JSON, пробую снова со строгим промптом")
	strictSystem := "Return ONLY a valid JSON object as specified. No other text, no markdown. All content in Russian."
	content, err = g.callAPI(strictSystem, fmt.Sprintf("Заголовок: %s\nОписание: %s", title, description), 800)
	if err != nil {
		return "", err
	}

	jsonStr, err = extractJSON(content)
	if err != nil {
		return "", fmt.Errorf("no JSON found in response even after strict prompt: %s", content)
	}
	jsonStr = strings.TrimSpace(jsonStr)
	if idx := strings.Index(jsonStr, "{"); idx != -1 {
		jsonStr = jsonStr[idx:]
		if end := strings.LastIndex(jsonStr, "}"); end != -1 {
			jsonStr = jsonStr[:end+1]
		}
	}
	return jsonStr, nil
}

// NewsItem — структура для новости, возвращаемой ИИ
type NewsItem struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Link        string `json:"link"`
}

// generator/generator.go

// FetchTrendingNews запрашивает у Groq/OpenRouter трендовые новости для детей 9-13 лет.
func (g *OpenRouterGenerator) FetchTrendingNews() ([]NewsItem, error) {
	systemPrompt := `Ты — редактор суперпопулярного детского канала (аудитория 9-13 лет).
Найди 3 САМЫЕ ОБСУЖДАЕМЫЕ И ВИРУСНЫЕ новости в России прямо сейчас, которые точно заинтересуют детей этого возраста.
Запрещено: скучные официальные новости, политика, экономика, взрослые темы.
Обязательно: мемы, тренды TikTok/YouTube, игры (Minecraft, Roblox, Brawl Stars), аниме, необычные челленджи, смешные ситуации, научные открытия, крутые гаджеты.
Верни СТРОГО JSON-массив объектов с полями title (заголовок), description (краткое описание) и link (ссылка на источник).`

	content, err := g.callAPI(systemPrompt, "Самые вирусные новости для школьников прямо сейчас", 600)
	if err != nil {
		return nil, err
	}
	log.Printf("Сырой ответ от Groq (тренды):\n%s", content)
	return parseNewsJSON(content)
}

func (g *OpenRouterGenerator) FetchTrendingNewsFallback() ([]NewsItem, error) {
	systemPrompt := `Ты ищешь новости для детского канала. Темы: новые мемы, тренды TikTok, обновления игр, аниме, челленджи, смешные истории из школ, необычные животные, крутые изобретения.
Верни JSON-массив с полями title, description, link.`

	content, err := g.callAPI(systemPrompt, "Что сегодня обсуждают дети 9-13 лет", 600)
	if err != nil {
		return nil, err
	}
	log.Printf("Сырой ответ от Groq (фолбэк):\n%s", content)
	return parseNewsJSON(content)
}

// parseNewsJSON собирает все JSON‑объекты новостей из ответа, даже если они разделены ][.
func parseNewsJSON(content string) ([]NewsItem, error) {
	jsonStr, err := extractJSON(content)
	if err != nil {
		return nil, fmt.Errorf("extractJSON: %w", err)
	}

	log.Printf("Сырой JSON от Groq:\n%s", jsonStr)

	// Склеиваем несколько массивов, разделённых ][, в один
	merged := strings.ReplaceAll(jsonStr, "][", ",")

	cleaned := cleanJSON(merged)

	var items []NewsItem
	if err := json.Unmarshal([]byte(cleaned), &items); err != nil {
		// Если массив не получилось распарсить, пробуем извлечь все объекты вручную
		items = extractObjects(cleaned)
		if len(items) > 0 {
			return items, nil
		}
		return nil, fmt.Errorf("unmarshal news items: %w (cleaned: %s)", err, cleaned)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("пустой список новостей")
	}
	return items, nil
}

// cleanJSON удаляет висячие запятые и всё после последней ']'
func cleanJSON(raw string) string {
	s := strings.ReplaceAll(raw, ",]", "]")
	s = strings.ReplaceAll(s, ",}", "}")
	s = strings.TrimRight(s, " \t\n\r")
	if strings.HasSuffix(s, ",") {
		s = s[:len(s)-1]
	}
	if idx := strings.LastIndex(s, "]"); idx != -1 {
		s = s[:idx+1]
	}
	// Убираем переносы строк и лишние пробелы
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\t", "")
	// Заменяем два и более пробелов на один
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return s
}

// extractObjects извлекает все JSON‑объекты из строки, даже если они не в массиве
func extractObjects(s string) []NewsItem {
	var items []NewsItem
	depth := 0
	start := -1
	for i, ch := range s {
		if ch == '{' {
			if depth == 0 {
				start = i
			}
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 && start != -1 {
				objStr := s[start : i+1]
				var item NewsItem
				if err := json.Unmarshal([]byte(objStr), &item); err == nil {
					items = append(items, item)
				}
				start = -1
			}
		}
	}
	return items
}

// callAPI отправляет запрос и возвращает содержимое ответа.
func (g *OpenRouterGenerator) callAPI(system, user string, maxTokens int) (string, error) {
	if maxTokens <= 0 {
		maxTokens = 500 // разумное значение по умолчанию
	}
	reqBody := map[string]interface{}{
		"model": g.Model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"temperature": 0.9,
		"max_tokens":  maxTokens,
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
	systemPrompt := `Ты — помощник для поиска картинок. Извлеки из новости 5-7 КЛЮЧЕВЫХ СЛОВ на РУССКОМ языке, которые лучше всего описывают её тему.
Это могут быть: имена собственные, названия игр, аниме, мемов, персонажей, действий, объектов.
НЕ включай: приветствия, общие фразы ("новость дня", "смотрите"), предлоги, союзы.
Верни СТРОГО JSON-массив строк.`

	userPrompt := fmt.Sprintf("Заголовок: %s\nОписание: %s", title, description)

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

// Добавь этот метод в файл generator/generator.go, внутрь структуры OpenRouterGenerator

// GenerateImagePrompts генерирует 5 разных промптов для фоновых изображений.
func (g *OpenRouterGenerator) GenerateImagePrompts(title, script string) ([]string, error) {
	systemPrompt := `Ты — креативный художник детского канала (аудитория 7-13 лет).
Придумай 5 разных промптов для генерации фоновых изображений к видео-новости.
Каждый промпт должен описывать отдельную сцену, связанную с темой новости.
Промпты должны быть на РУССКОМ языке, яркими, позитивными, без жестокости.
Разнообразие: разные ракурсы, действия, эмоции, детали.
Верни СТРОГО JSON-массив из 5 строк, без markdown. Пример: ["промпт1", "промпт2", "промпт3", "промпт4", "промпт5"]`

	userPrompt := fmt.Sprintf("Заголовок: %s\nСценарий: %s", title, script)

	reqBody := map[string]interface{}{
		"model": g.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0.9,
		"max_tokens":  300,
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
	// Очищаем от маркёров ```json ... ```
	cleaned := strings.ReplaceAll(strings.ReplaceAll(content, "```json", ""), "```", "")
	cleaned = strings.TrimSpace(cleaned)

	// Ищем JSON-массив
	start := strings.Index(cleaned, "[")
	end := strings.LastIndex(cleaned, "]")
	if start == -1 || end == -1 || start >= end {
		return nil, fmt.Errorf("no JSON array found in response: %s", content)
	}

	var prompts []string
	if err := json.Unmarshal([]byte(cleaned[start:end+1]), &prompts); err != nil {
		return nil, fmt.Errorf("unmarshal prompts: %w", err)
	}

	if len(prompts) < 3 {
		return nil, fmt.Errorf("too few prompts returned: %d", len(prompts))
	}

	return prompts[:5], nil
}

// GenerateImage генерирует картинку по текстовому промпту через OpenRouter.
func (g *OpenRouterGenerator) GenerateImage(prompt string) (string, error) {
	// Используем модель google/gemini-3-pro-image-preview для генерации изображений
	reqBody := map[string]interface{}{
		"model": "google/gemini-3-pro-image-preview",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.9,
		"max_tokens":  500,
	}

	jsonData, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.APIKey)
	req.Header.Set("HTTP-Referer", "https://github.com/your-app")
	req.Header.Set("X-Title", "KidsShortsGenerator")

	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("OpenRouter image request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OpenRouter image API error %d: %s", resp.StatusCode, string(body))
	}

	// Парсим ответ – изображение придёт в формате base64 внутри JSON
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode image response: %w", err)
	}
	if len(result.Choices) == 0 || result.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("empty image response")
	}

	// Декодируем base64 → JPEG
	imgBytes, err := base64.StdEncoding.DecodeString(result.Choices[0].Message.Content)
	if err != nil {
		return "", fmt.Errorf("decode base64 image: %w", err)
	}

	os.MkdirAll("backgrounds", 0755)
	filename := filepath.Join("backgrounds", fmt.Sprintf("openrouter_img_%d.jpg", time.Now().UnixNano()))
	if err := os.WriteFile(filename, imgBytes, 0644); err != nil {
		return "", fmt.Errorf("write image file: %w", err)
	}
	return filename, nil
}
