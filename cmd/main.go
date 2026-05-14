// cmd/main.go
package main

import (
	"encoding/json"
	"fmt"
	"log"
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

type Script struct {
	FullText  string   `json:"full_text"`
	Subtitles []string `json:"subtitles"`
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
		MaxArticles: 5,
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
	os.MkdirAll("output", 0755)
	os.MkdirAll(filepath.Join("output", "audio"), 0755)

	gen := generator.NewOpenRouter(os.Getenv("OPENROUTER_API_KEY"))

	var (
		wg      sync.WaitGroup
		dedupMu sync.Mutex
	)

	for i, article := range articles {
		wg.Add(1)
		go func(idx int, art collector.Article) {
			defer wg.Done()

			if !isKidSafe(art.Title, art.Description) {
				log.Printf("Пропущена неподходящая статья %d: %s", idx+1, art.Title)
				return
			}
			scriptJSON, err := gen.GenerateScript(art.Title, art.Description)
			if err != nil {
				log.Printf("Ошибка генерации для статьи %d: %v", idx+1, err)
				return
			}
			fmt.Printf("Сгенерированный сценарий для статьи %d:\n%s\n\n", idx+1, scriptJSON)

			var script Script
			if err := json.Unmarshal([]byte(scriptJSON), &script); err != nil {
				log.Printf("Ошибка парсинга сценария %d: %v", idx+1, err)
				return
			}

			audioPath, err := saluteClient.Synthesize(script.FullText)
			if err != nil {
				log.Printf("Ошибка озвучки статьи %d: %v", idx+1, err)
				return
			}

			// Уникальное имя аудио, чтобы горутины не перезаписывали файлы
			uniqueAudio := filepath.Join("output", "audio", fmt.Sprintf("article_%d_%s", idx+1, filepath.Base(audioPath)))
			if err := os.Rename(audioPath, uniqueAudio); err != nil {
				log.Printf("Ошибка перемещения аудио %d: %v", idx+1, err)
				return
			}
			log.Printf("Статья %d озвучена: %s", idx+1, uniqueAudio)

			videoOutput := filepath.Join("output", fmt.Sprintf("video_%d.mp4", idx+1))

			// Фоновое видео
			keywords := compositor.ExtractKeywords(art.Title) + fmt.Sprintf(" %d", time.Now().UnixNano()%100)
			if keywords == "" {
				keywords = "minecraft gameplay"
			}
			bgVideo, err := compositor.FetchStockVideo(os.Getenv("PEXELS_API_KEY"), keywords, nil)
			if err != nil {
				log.Printf("Стоковое видео не найдено для статьи %d: %v (использую чёрный фон)", idx+1, err)
			} else {
				log.Printf("Стоковое видео скачано для статьи %d: %s", idx+1, bgVideo)
			}

			if err := compositor.ComposeVertical(compositor.ComposeParams{
				AudioPath:  uniqueAudio,
				Subtitles:  script.Subtitles,
				OutputPath: videoOutput,
			}, bgVideo); err != nil {
				log.Printf("Ошибка сборки видео для статьи %d: %v", idx+1, err)
				return
			}
			log.Printf("Видео для статьи %d собрано: %s", idx+1, videoOutput)

			dedupMu.Lock()
			if err := dedup.MarkPublished(art.Link); err != nil {
				log.Printf("Ошибка сохранения ссылки %d: %v", idx+1, err)
			}
			dedupMu.Unlock()
		}(i, article)
	}

	wg.Wait()
	fmt.Println("Все статьи обработаны.")
}
