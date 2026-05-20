// cmd/main.go
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Hoher2000/school_trends/collector"
	"github.com/Hoher2000/school_trends/compositor"
	"github.com/Hoher2000/school_trends/generator"
	"github.com/Hoher2000/school_trends/publisher"
	"github.com/Hoher2000/school_trends/tts"
	"github.com/Hoher2000/school_trends/utils"
	"github.com/joho/godotenv"
	"github.com/mailru/easyjson"
)

func main() {
	projectRoot := utils.ProjectRoot()
	_ = godotenv.Load(filepath.Join(projectRoot, ".env"))
	actualTime := fmt.Sprintf(" %s %d", time.Now().Month().String(), time.Now().Year())
	dbPath := filepath.Join(projectRoot, "bolt.db")

	dedup, err := collector.NewDeduplicator(dbPath, 10*24*time.Hour)
	if err != nil {
		log.Fatalf("dedup init: %v", err)
	}
	defer dedup.Close()

	if err := dedup.CleanupOldEntries(30 * 24 * time.Hour); err != nil {
		log.Printf("Ошибка очистки старых записей: %v", err)
	}

	tm := tts.NewTokenManager(
		os.Getenv("SALUTE_CLIENT_ID"),
		os.Getenv("SALUTE_CLIENT_SECRET"),
		"SALUTE_SPEECH_PERS",
		"https://ngw.devices.sberbank.ru:9443/api/v2/oauth",
	)
	if err := tm.Start(); err != nil {
		log.Fatalf("TokenManager: %v", err)
	}
	defer tm.Stop()

	saluteClient, err := tts.NewSaluteClient(tm)
	if err != nil {
		log.Fatalf("salute init: %v", err)
	}
	defer saluteClient.Close()

	ytKey := os.Getenv("YOUTUBE_API_KEY")
	newsQueries := []string{
		`Minecraft OR Roblox`,
		`Brawl Stars OR "Adopt Me" OR "Brookhaven"`,
		`"новые игры" дети OR подростки`,
		`аниме OR "Моя геройская академия"`,
		`мемы смешные для детей 10-13 лет`,
		`"Likee"`,
		`"популярные блогеры" дети`,
		`"тренды ХАЙП" дети 9-13 лет`,
		`"six seven" дети`,
	}
	for i := range newsQueries {
		newsQueries[i] += actualTime
	}
	params := collector.CollectParams{
		NewsQueries:    newsQueries,
		YouTubeApiKey:  ytKey,
		YouTubeQueries: newsQueries,
		MaxArticles:    1,
		Dedup:          dedup,
	}

	articles, err := collector.CollectTrends(params)
	if err != nil {
		log.Fatalf("collect error: %v", err)
	}

	fmt.Println("Собрано статей:", len(articles))
	for i, art := range articles {
		fmt.Printf("%d. [%s] %s\n   %s\n", i+1, art.Source, art.Title, art.Link)
	}

	// Создаём папки один раз до горутин
	os.MkdirAll("backgrounds", 0755)
	os.MkdirAll("output", 0755)
	os.MkdirAll(filepath.Join("output", "audio"), 0755)

	gen := generator.NewOpenRouter(os.Getenv("OPENROUTER_API_KEY"))
	// GigaChat-генератор промптов (опционально)
	/*gigachatGen, err := generator.NewGigaChatGenerator(os.Getenv("GIGACHAT_API_KEY"))
	if err != nil {
		log.Printf("Не удалось создать GigaChat генератор: %v", err)
	} else {
		defer gigachatGen.Close()
	}*/
	tgToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	tgChatID := os.Getenv("TELEGRAM_CHAT_ID")

	for idx, art := range articles {
		if err := dedup.MarkPublished(art.Link); err != nil {
			log.Printf("Ошибка сохранения заголовка %d: %v", idx+1, err)
		} // сохраняем, чтобы не повторять

		// Проверка на недетский контент
		if !utils.IsKidSafe(art.Title, art.Description) {
			log.Printf("Пропущена неподходящая статья %d: %s", idx+1, art.Title)
			continue
		}

		// Генерация сценария
		scriptJSON, err := gen.GenerateScript(art.Title, art.Description)
		if err != nil {
			log.Printf("Ошибка генерации для статьи %d: %v", idx+1, err)
			continue
		}

		fmt.Printf("Сгенерированный сценарий для статьи %d:\n%s\n\n", idx+1, scriptJSON)

		script := &generator.Script{}
		if err := easyjson.Unmarshal([]byte(scriptJSON), script); err != nil {
			log.Printf("Ошибка парсинга сценария %d: %v", idx+1, err)
			continue
		}

		// Проверка маркера skip (невозрастной контент)
		if script.Skip {
			log.Printf("Статья %d не подходит для детей: %s", idx+1, art.Title)
			continue
		}

		var uniqueAudioPath string
		saluteErrorChan := make(chan error)
		go func(uap *string, ch chan error) {
			var err error
			var audioPath string
			audioPath, err = saluteClient.Synthesize(script.FullText)
			if err != nil {
				log.Printf("Ошибка озвучки статьи %d: %v", idx+1, err)
				ch <- err
			}

			// Уникальное имя аудио
			*uap = filepath.Join("output", "audio",
				fmt.Sprintf("article_%d_%s", idx+1, filepath.Base(audioPath)))
			if err := os.Rename(audioPath, *uap); err != nil {
				log.Printf("Ошибка перемещения аудио %d: %v", idx+1, err)
				ch <- err
			}
			log.Printf("Статья %d озвучена: %s", idx+1, *uap)
			ch <- nil
		}(&uniqueAudioPath, saluteErrorChan)
		// Ключевые слова для фона
		var keywords string
		if aiKeywords, err := gen.ExtractKeywords(art.Title, art.Description); err == nil && len(aiKeywords) > 0 {
			keywords = strings.Join(aiKeywords, " ")
		} else {
			log.Printf("Ошибка получения ключевых слов от ИИ: %v", err)
			keywords = utils.ExtractKeywordsFromTitle(art.Title)
		}
		if keywords == "" {
			words := strings.Fields(script.FullText)
			if len(words) > 3 {
				keywords = utils.Transliterate(strings.Join(words[3:], " "))
			} else {
				keywords = utils.Transliterate(script.FullText)
			}
		}
		if keywords == "" {
			keywords = compositor.ExtractKeywords(art.Title)
		}
		if keywords == "" {
			keywords = "minecraft gameplay"
		}
		keywords = fmt.Sprintf("%s %d", keywords, time.Now().UnixNano()%100)
		fmt.Println("Ключевые слова для фона:", keywords)

		sources := make(map[string]string)

		/*//1. Pexels (стоковое видео) — закомментировано, но оставлено для быстрого включения
		if pexelsVideo, err := compositor.FetchStockVideo(os.Getenv("PEXELS_API_KEY"), keywords, nil); err == nil {
			sources["pexels"] = pexelsVideo
			log.Printf("Pexels видео скачано: %s", pexelsVideo)
		} else {
			log.Printf("Pexels: %v", err)
		}*/

		// Промпт для Kandinsky Video (опционально)
		/*if gigachatGen != nil {
			if _, err := gigachatGen.GenerateKandinskyPrompt(art.Title, art.Description); err != nil {
				log.Printf("Ошибка генерации промпта для Kandinsky: %v", err)
			}
		}*/

		// Яндекс.Картинки
		// Задержка, чтобы не упереться в лимит OpenSERP при параллельных запросах
		yandexQuery := utils.ExtractRussianKeywords(art.Title)
		if yandexQuery == "" {
			yandexQuery = strings.TrimSpace(art.Title)
		}
		if yandexQuery != "" {
			if yandexImages, err := collector.FetchYandexImages(yandexQuery, 10); err == nil && len(yandexImages) > 0 {
				log.Printf("Найдено %d картинок Яндекса", len(yandexImages))
				downloadedImages := utils.DownloadImagesConcurrently(yandexImages, fmt.Sprintf("yandex_%d", idx+1), 10*time.Second, 5)

				if len(downloadedImages) < 10 {
					fallbackQuery := "яркие картинки дети"
					if fbImages, err := collector.FetchYandexImages(fallbackQuery, 20); err == nil {
						moreImages := utils.DownloadImagesConcurrently(fbImages, fmt.Sprintf("yandex_fb_%d", idx+1), 10*time.Second, 5)
						downloadedImages = append(downloadedImages, moreImages...)
					}
				}
				if len(downloadedImages) > 0 {
					slideshowVideo := filepath.Join("output", fmt.Sprintf("slideshow_yandex_%d.mp4", idx+1))
					if err := compositor.CreateSlideshow(downloadedImages, slideshowVideo); err == nil {
						sources["yandex_images"] = slideshowVideo
					} else {
						log.Printf("Слайдшоу из Яндекса: %v", err)
					}
				}
			} else {
				log.Printf("Яндекс.Картинки: %v", err)
			}
		}

		// Генерация 5 фоновых картинок через Pollinations (последовательно, с повторными попытками)
		/*if true {
			var genImages []string

			prompts := []string{
				fmt.Sprintf("Colorful cartoon illustration for kids about %s, bright colors, fun and engaging", art.Title),
				fmt.Sprintf("Playful background for children video about %s, children's book style, happy mood", art.Title),
				fmt.Sprintf("Dynamic action scene for kids video about %s, adventure, vibrant colors", art.Title),
				fmt.Sprintf("Smiling characters illustration for kids video about %s, cute, flat design", art.Title),
				fmt.Sprintf("Educational style illustration for kids video about %s, simple, clean, colorful", art.Title),
			}

			for _, prompt := range prompts {
				imgPath, err := collector.FetchPollinationsImageWithRetry(prompt, 3) // до 3 попыток
				if err != nil {
					log.Printf("Pollinations ошибка генерации: %v", err)
					continue
				}
				genImages = append(genImages, imgPath)
				log.Printf("Pollinations изображение сгенерировано: %s", imgPath)
				time.Sleep(2 * time.Second) // задержка между запросами, чтобы не перегружать
			}

			if len(genImages) >= 3 {
				slideshowVideo := filepath.Join("output", fmt.Sprintf("slideshow_pollinations_%d.mp4", idx+1))
				if err := compositor.CreateSlideshow(genImages, slideshowVideo); err == nil {
					sources["pollinations"] = slideshowVideo
					log.Printf("Pollinations слайд-шоу собрано: %s", slideshowVideo)
				} else {
					log.Printf("Ошибка сборки Pollinations слайд-шоу: %v", err)
				}
			} else {
				log.Printf("Pollinations: сгенерировано недостаточно изображений для слайд-шоу (%d из 5)", len(genImages))
			}
		}*/

		// Сборка видео для каждого источника
		if err := <-saluteErrorChan; err != nil {
			continue
		}
		for source, bgPath := range sources {
			if bgPath == "" {
				continue
			}
			videoOutput := filepath.Join("output", fmt.Sprintf("video_%s_%d.mp4", source, idx+1))
			if err := compositor.ComposeVertical(compositor.ComposeParams{
				AudioPath:  uniqueAudioPath,
				Subtitles:  script.Subtitles,
				OutputPath: videoOutput,
			}, bgPath); err != nil {
				log.Printf("Ошибка сборки %s: %v", source, err)
			} else {
				fmt.Printf("✅ Видео %s собрано: %s\n", source, videoOutput)

				//отсылка в телеграмм
				if tgToken != "" && tgChatID != "" {
					caption := fmt.Sprintf("📰 %s\n\n%s", art.Title, script.FullText)
					if err := publisher.SendVideoToTelegram(tgToken, tgChatID, videoOutput, caption); err != nil {
						log.Printf("Ошибка отправки в Telegram: %v", err)
					} else {
						log.Printf("Видео отправлено в Telegram: %s", videoOutput)
					}
				}

				//отсылка в вк группу (не работает из за разного айпи кодеспейс и полученного токена)
				/*if vkToken := os.Getenv("VK_ACCESS_TOKEN"); vkToken != "" {
					if vkGroupID := os.Getenv("VK_GROUP_ID"); vkGroupID != "" {
						caption := fmt.Sprintf("%s\n\n%s", art.Title, script.FullText)
						if err := publisher.PostVideo(vkToken, vkGroupID, videoOutput, caption); err != nil {
							log.Printf("Ошибка публикации в VK: %v", err)
						} else {
							log.Printf("Видео опубликовано в VK: %s", videoOutput)
						}
					}
				}*/
			}
		}
	}
	fmt.Println("Все статьи обработаны.")
}
