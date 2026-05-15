package generator

import (
	"context"
	"fmt"
	"log"

	"github.com/AlexandrVIvanov/gigago"
)

// GigaChatGenerator генерирует промпт для Kandinsky Video через GigaChat API.
type GigaChatGenerator struct {
	client *gigago.Client
	model  *gigago.GenerativeModel
}

// NewGigaChatGenerator создаёт новый генератор.
func NewGigaChatGenerator(apiKey string) (*GigaChatGenerator, error) {
	// 1. Создаём клиент, который сам управляет токенами.
	client, err := gigago.NewClient(
		context.Background(),
		apiKey,
		gigago.WithCustomInsecureSkipVerify(true), // обязательно для обхода проблем с сертификатами
	)
	if err != nil {
		return nil, fmt.Errorf("не удалось создать клиент GigaChat: %w", err)
	}

	// 2. Выбираем модель. GigaChat-Pro — мощная и креативная.
	model := client.GenerativeModel("GigaChat-Pro")

	return &GigaChatGenerator{
		client: client,
		model:  model,
	}, nil
}

// Close закрывает клиент и останавливает фоновое обновление токена.
func (g *GigaChatGenerator) Close() {
	g.client.Close()
}

// GenerateKandinskyPrompt создаёт промпт для Kandinsky Video на основе новости.
func (g *GigaChatGenerator) GenerateKandinskyPrompt(title, description string) (string, error) {
	// 3. Составляем инструкцию для модели.
	systemPrompt := `Ты — креативный режиссер детского канала (аудитория 7-13 лет).
Создай краткий промпт на РУССКОМ языке для генерации 5-секундного вертикального видео (9:16) по новости.
Промпт должен быть на РУССКОМ языке и описывать сцену, которая идеально подходит в качестве фона для этой новости.
Используй яркие и позитивные образы: красочный, мультяшный, веселый, динамичный.`

	userPrompt := fmt.Sprintf("Заголовок новости: %s\nОписание: %s", title, description)

	// 4. Отправляем запрос.
	messages := []gigago.Message{
		{Role: gigago.RoleSystem, Content: systemPrompt},
		{Role: gigago.RoleUser, Content: userPrompt},
	}

	resp, err := g.model.Generate(context.Background(), messages)
	if err != nil {
		return "", fmt.Errorf("ошибка генерации промпта: %w", err)
	}

	if len(resp.Choices) > 0 {
		prompt := resp.Choices[0].Message.Content
		log.Printf("Сгенерирован промпт для Kandinsky: %s", prompt)
		return prompt, nil
	}

	return "", fmt.Errorf("пустой ответ от GigaChat")
}
