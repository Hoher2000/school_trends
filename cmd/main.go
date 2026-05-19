// cmd/main.go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Hoher2000/school_trends/collector"
	"github.com/Hoher2000/school_trends/compositor"
	"github.com/Hoher2000/school_trends/generator"
	"github.com/Hoher2000/school_trends/tts"
	"github.com/joho/godotenv"
)

type Script struct {
	FullText  string   `json:"full_text"`
	Subtitles []string `json:"subtitles"`
	Skip      bool     `json:"skip,omitempty"`
}

func projectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "."
}

func isKidSafe(title, description string) bool {
	stopWords := []string{
		"убил", "застрелили", "смерть", "погиб", "трагедия", "катастрофа",
		"взорвал", "террорист", "жесток", "насилие", "ограбление", "изнасилование",
		"наркотик", "алкоголь", "взрослый контент", "18+", "эротик", "порно",
		"азартные игры", "казино", "ставки на спорт",
	}
	combined := strings.ToLower(title + " " + description)
	for _, word := range stopWords {
		if strings.Contains(combined, word) {
			return false
		}
	}
	return true
}

func main() {
	_ = godotenv.Load(filepath.Join(projectRoot(), ".env"))
	currentYear := time.Now().Year()
	dbPath := filepath.Join(projectRoot(), "bolt.db")
	dedup, err := collector.NewDeduplicator(dbPath)
	if err != nil {
		log.Fatalf("dedup init: %v", err)
	}
	defer dedup.Close()

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

	params := collector.CollectParams{
		NewsQueries: []string{
			`Minecraft OR Roblox`,
			`Brawl Stars OR "Adopt Me" OR "Brookhaven"`,
			fmt.Sprintf(`"новые игры" дети OR подростки %d`, currentYear),
			fmt.Sprintf(`аниме %d OR "Моя геройская академия"`, currentYear),
			fmt.Sprintf(`мемы %d смешные`, currentYear),
			`"Sigma Boy" OR "Гном Гномыч" OR "Likee"`,
			`"популярные блогеры" дети`,
			`"школьники" новости интересные`,
			`"six seven" дети`,
		},
		YouTubeApiKey: ytKey,
		YouTubeQueries: []string{
			fmt.Sprintf("обзор Minecraft %d", currentYear),
			fmt.Sprintf("аниме топ %d", currentYear),
		},
		MaxArticles: 1,
		Dedup:       dedup,
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
	gigachatGen, err := generator.NewGigaChatGenerator(os.Getenv("GIGACHAT_API_KEY"))
	if err != nil {
		log.Printf("Не удалось создать GigaChat генератор: %v", err)
	} else {
		defer gigachatGen.Close()
	}
	// Бесплатный клиент для генерации музыки через Hugging Face Gradio
	//hfMusicClient := collector.NewHuggingFaceMusicClient()

	var (
		wg      sync.WaitGroup
		dedupMu sync.Mutex
		synMu   sync.Mutex // ← новый мьютекс для синтеза речи
	)

	for i, article := range articles {
		wg.Add(1)
		go func(idx int, art collector.Article) {
			defer wg.Done()

			// Проверка на недетский контент
			if !isKidSafe(art.Title, art.Description) {
				log.Printf("Пропущена неподходящая статья %d: %s", idx+1, art.Title)
				dedupMu.Lock()
				dedup.MarkPublished(art.Title) // сохраняем, чтобы не повторять
				dedupMu.Unlock()
				return
			}

			// Генерация сценария
			scriptJSON, err := gen.GenerateScript(art.Title, art.Description)
			if err != nil {
				log.Printf("Ошибка генерации для статьи %d: %v", idx+1, err)
				dedupMu.Lock()
				dedup.MarkPublished(art.Title)
				dedupMu.Unlock()
				return
			}
			fmt.Printf("Сгенерированный сценарий для статьи %d:\n%s\n\n", idx+1, scriptJSON)

			var script Script
			if err := json.Unmarshal([]byte(scriptJSON), &script); err != nil {
				log.Printf("Ошибка парсинга сценария %d: %v", idx+1, err)
				dedupMu.Lock()
				dedup.MarkPublished(art.Title)
				dedupMu.Unlock()
				return
			}

			// Проверка маркера skip (невозрастной контент)
			if script.Skip {
				log.Printf("Статья %d не подходит для детей: %s", idx+1, art.Title)
				dedupMu.Lock()
				dedup.MarkPublished(art.Title)
				dedupMu.Unlock()
				return
			}

			// Озвучка (защищена мьютексом, чтобы имена файлов не пересекались)
			synMu.Lock()
			audioPath, err := saluteClient.Synthesize(script.FullText)
			if err != nil {
				synMu.Unlock()
				log.Printf("Ошибка озвучки статьи %d: %v", idx+1, err)
				dedupMu.Lock()
				dedup.MarkPublished(art.Title)
				dedupMu.Unlock()
				return
			}

			// Уникальное имя аудио
			uniqueAudio := filepath.Join("output", "audio",
				fmt.Sprintf("article_%d_%s", idx+1, filepath.Base(audioPath)))
			if err := os.Rename(audioPath, uniqueAudio); err != nil {
				synMu.Unlock()
				log.Printf("Ошибка перемещения аудио %d: %v", idx+1, err)
				dedupMu.Lock()
				dedup.MarkPublished(art.Title)
				dedupMu.Unlock()
				return
			}
			synMu.Unlock()
			log.Printf("Статья %d озвучена: %s", idx+1, uniqueAudio)

			// Ключевые слова для фона
			var keywords string
			if aiKeywords, err := gen.ExtractKeywords(art.Title, art.Description); err == nil && len(aiKeywords) > 0 {
				keywords = strings.Join(aiKeywords, " ")
			} else {
				log.Printf("Ошибка получения ключевых слов от ИИ: %v", err)
				keywords = extractKeywordsFromTitle(art.Title)
			}
			if keywords == "" {
				words := strings.Fields(script.FullText)
				if len(words) > 3 {
					keywords = transliterate(strings.Join(words[3:], " "))
				} else {
					keywords = transliterate(script.FullText)
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

			//1. Pexels (стоковое видео) — закомментировано, но оставлено для быстрого включения
			/*if pexelsVideo, err := compositor.FetchStockVideo(os.Getenv("PEXELS_API_KEY"), keywords, nil); err == nil {
				sources["pexels"] = pexelsVideo
				log.Printf("Pexels видео скачано: %s", pexelsVideo)
			} else {
				log.Printf("Pexels: %v", err)
			}*/

			// Генерация фоновой музыки через Hugging Face Gradio
			/*if hfMusicClient != nil {
				audioDur, err := compositor.GetAudioDuration(uniqueAudio)
				if err == nil {
					durSec := int(audioDur.Seconds())
					if durSec < 5 {
						durSec = 5
					}
					if durSec > 30 {
						durSec = 30
					}
					musicPrompt := fmt.Sprintf("cheerful and playful instrumental background music for kids video about %s", art.Title)
					if musicFile, err := hfMusicClient.GenerateMusic(musicPrompt, durSec); err == nil {
						sources["ai_music"] = musicFile
						log.Printf("AI-музыка сгенерирована: %s", musicFile)
					} else {
						log.Printf("AI-музыка не сгенерирована: %v", err)
					}
				} else {
					log.Printf("Не удалось определить длительность аудио: %v", err)
				}
			}*/

			// Промпт для Kandinsky Video (опционально)
			/*if gigachatGen != nil {
				if _, err := gigachatGen.GenerateKandinskyPrompt(art.Title, art.Description); err != nil {
					log.Printf("Ошибка генерации промпта для Kandinsky: %v", err)
				}
			}*/

			// Яндекс.Картинки
			// Задержка, чтобы не упереться в лимит OpenSERP при параллельных запросах
			time.Sleep(2 * time.Second)
			yandexQuery := extractRussianKeywords(art.Title)
			if yandexQuery == "" {
				yandexQuery = strings.TrimSpace(art.Title)
			}
			if yandexQuery != "" {
				if yandexImages, err := collector.FetchYandexImages(yandexQuery, 10); err == nil && len(yandexImages) > 0 {
					log.Printf("Найдено %d картинок Яндекса", len(yandexImages))
					downloadedImages := downloadImagesConcurrently(yandexImages, fmt.Sprintf("yandex_%d", idx+1), 10*time.Second, 5)

					if len(downloadedImages) < 10 {
						fallbackQuery := "яркие картинки дети"
						if fbImages, err := collector.FetchYandexImages(fallbackQuery, 20); err == nil {
							moreImages := downloadImagesConcurrently(fbImages, fmt.Sprintf("yandex_fb_%d", idx+1), 10*time.Second, 5)
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

			/*
				// Генерация 5 фоновых картинок через Pollinations (последовательно, с повторными попытками)
				if true {
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
				}
			*/
			// Сборка видео для каждого источника
			for source, bgPath := range sources {
				if bgPath == "" {
					continue
				}
				videoOutput := filepath.Join("output", fmt.Sprintf("video_%s_%d.mp4", source, idx+1))
				if err := compositor.ComposeVertical(compositor.ComposeParams{
					AudioPath:  uniqueAudio,
					Subtitles:  script.Subtitles,
					OutputPath: videoOutput,
				}, bgPath); err != nil {
					log.Printf("Ошибка сборки %s: %v", source, err)
				} else {
					fmt.Printf("✅ Видео %s собрано: %s\n", source, videoOutput)
				}
			}

			// Отчёт о пропущенных источниках
			missing := []string{}
			for _, name := range []string{"youtube", "rutube", "vk", "images", "pexels"} {
				if _, ok := sources[name]; !ok {
					missing = append(missing, name)
				}
			}
			if len(missing) > 0 {
				fmt.Printf("⚠️ Не удалось получить фоны из: %v\n", missing)
			}
			fmt.Println("\nГотово! Все варианты в папке output/. Выберите лучший.")

			// Сохраняем статью как обработанную
			dedupMu.Lock()
			if err := dedup.MarkPublished(art.Title); err != nil {
				log.Printf("Ошибка сохранения заголовка %d: %v", idx+1, err)
			}
			dedupMu.Unlock()
		}(i, article)
	}

	wg.Wait()
	fmt.Println("Все статьи обработаны.")
}

// --- Вспомогательные функции ---

var stopWords = map[string]bool{
	"и": true, "в": true, "на": true, "с": true, "по": true, "для": true, "от": true, "к": true,
	"у": true, "за": true, "из": true, "до": true, "об": true, "под": true, "над": true,
	"перед": true, "при": true, "про": true, "через": true, "без": true, "не": true, "но": true,
	"а": true, "или": true, "как": true, "что": true, "чтобы": true, "это": true, "то": true,
	"он": true, "она": true, "они": true, "мы": true, "вы": true, "ты": true, "я": true,
	"меня": true, "мне": true, "мой": true, "твой": true, "свой": true, "его": true, "её": true,
	"их": true, "весь": true, "вся": true, "всё": true, "все": true, "который": true,
	"которая": true, "которые": true, "быть": true, "есть": true, "был": true, "была": true,
	"было": true, "были": true, "будут": true, "будет": true, "сказать": true, "говорить": true,
	"мочь": true, "сделать": true, "ещё": true, "уже": true, "очень": true, "так": true,
	"вот": true, "там": true, "тут": true, "где": true, "когда": true, "почему": true,
	"какой": true, "такая": true, "также": true, "только": true, "даже": true, "просто": true,
	"более": true, "менее": true, "сейчас": true, "сегодня": true, "завтра": true, "вчера": true,
	"потом": true, "всегда": true, "никогда": true, "иногда": true, "вообще": true,
	"конечно": true, "пожалуйста": true, "извините": true, "привет": true, "пока": true,
	"здравствуйте": true, "до свидания": true,
	// Английские
	"the": true, "a": true, "an": true, "in": true, "on": true, "at": true, "to": true,
	"for": true, "of": true, "from": true, "by": true, "with": true, "about": true,
	"is": true, "are": true, "was": true, "were": true, "be": true, "been": true,
	"will": true, "would": true, "could": true, "should": true, "may": true, "might": true,
	"can": true, "shall": true, "has": true, "have": true, "had": true, "do": true,
	"does": true, "did": true, "and": true, "but": true, "or": true, "not": true,
	"no": true, "yes": true, "so": true, "if": true, "then": true, "else": true,
	"when": true, "where": true, "why": true, "how": true, "all": true, "both": true,
	"each": true, "few": true, "more": true, "most": true, "other": true, "some": true,
	"such": true, "only": true, "own": true, "same": true, "than": true, "too": true,
	"very": true, "just": true, "now": true, "here": true, "there": true,
}

func extractKeywordsFromTitle(title string) string {
	var clean strings.Builder
	for _, r := range title {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= 'а' && r <= 'я') || (r >= 'А' && r <= 'Я') ||
			(r >= '0' && r <= '9') || r == ' ' {
			clean.WriteRune(r)
		}
	}
	words := strings.Fields(clean.String())
	var meaningful []string
	for _, w := range words {
		lower := strings.ToLower(w)
		if len(lower) <= 2 || stopWords[lower] {
			continue
		}
		meaningful = append(meaningful, lower)
	}
	if len(meaningful) > 5 {
		meaningful = meaningful[:5]
	}
	return transliterate(strings.Join(meaningful, " "))
}

func transliterate(s string) string {
	repl := strings.NewReplacer(
		"а", "a", "б", "b", "в", "v", "г", "g", "д", "d", "е", "e", "ё", "yo",
		"ж", "zh", "з", "z", "и", "i", "й", "y", "к", "k", "л", "l", "м", "m",
		"н", "n", "о", "o", "п", "p", "р", "r", "с", "s", "т", "t", "у", "u",
		"ф", "f", "х", "kh", "ц", "ts", "ч", "ch", "ш", "sh", "щ", "shch",
		"ъ", "", "ы", "y", "ь", "", "э", "e", "ю", "yu", "я", "ya",
		"А", "A", "Б", "B", "В", "V", "Г", "G", "Д", "D", "Е", "E", "Ё", "Yo",
		"Ж", "Zh", "З", "Z", "И", "I", "Й", "Y", "К", "K", "Л", "L", "М", "M",
		"Н", "N", "О", "O", "П", "P", "Р", "R", "С", "S", "Т", "T", "У", "U",
		"Ф", "F", "Х", "Kh", "Ц", "Ts", "Ч", "Ch", "Ш", "Sh", "Щ", "Shch",
		"Ъ", "", "Ы", "Y", "Ь", "", "Э", "E", "Ю", "Yu", "Я", "Ya",
	)
	return repl.Replace(s)
}

func extractRussianKeywords(title string) string {
	var clean strings.Builder
	for _, r := range title {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= 'а' && r <= 'я') || (r >= 'А' && r <= 'Я') ||
			(r >= '0' && r <= '9') || r == ' ' {
			clean.WriteRune(r)
		}
	}
	words := strings.Fields(clean.String())
	var meaningful []string
	for _, w := range words {
		lower := strings.ToLower(w)
		if len(lower) <= 2 || stopWords[lower] {
			continue
		}
		meaningful = append(meaningful, lower)
	}
	if len(meaningful) > 5 {
		meaningful = meaningful[:5]
	}
	return strings.Join(meaningful, " ")
}

func downloadImageWithTimeout(url, filepath string, timeout time.Duration) error {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %d", resp.StatusCode)
	}
	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		os.Remove(filepath)
		return fmt.Errorf("stat: %w", err)
	}
	if info.Size() < 1024 {
		os.Remove(filepath)
		return fmt.Errorf("file too small (%d bytes)", info.Size())
	}
	return nil
}

func downloadImagesConcurrently(urls []string, prefix string, timeout time.Duration, maxConcurrent int) []string {
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []string
		sem     = make(chan struct{}, maxConcurrent)
	)
	for i, imgURL := range urls {
		wg.Add(1)
		go func(idx int, url string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			destFile := filepath.Join("backgrounds", fmt.Sprintf("%s_%d.jpg", prefix, idx))
			if err := downloadImageWithTimeout(url, destFile, timeout); err != nil {
				log.Printf("Не удалось скачать %s: %v", url, err)
				return
			}
			mu.Lock()
			results = append(results, destFile)
			mu.Unlock()
			log.Printf("Скачано (%d/%d): %s", len(results), len(urls), destFile)
		}(i, imgURL)
	}
	wg.Wait()
	return results
}
