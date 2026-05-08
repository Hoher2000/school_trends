// cmd/main.go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
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
			break // дошли до корня файловой системы
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
	// Создаем TokenManager
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
		},
		YouTubeApiKey: ytKey,
		YouTubeQueries: []string{
			fmt.Sprintf("обзор Minecraft %d", currentYear),
			fmt.Sprintf("аниме топ %d", currentYear),
		},
		MaxArticles: 2,
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

	gen := generator.NewOpenRouter(os.Getenv("OPENROUTER_API_KEY"))

	for i, article := range articles {
		if !isKidSafe(article.Title, article.Description) {
			log.Printf("Пропущена неподходящая статья %d: %s", i+1, article.Title)
			continue
		}
		scriptJSON, err := gen.GenerateScript(article.Title, article.Description)
		if err != nil {
			log.Printf("Ошибка генерации для статьи %d: %v", i+1, err)
			continue
		}
		fmt.Printf("Сгенерированный сценарий для статьи %d:\n%s\n\n", i+1, scriptJSON)

		// Парсим JSON в структуру Script
		var script Script
		if err := json.Unmarshal([]byte(scriptJSON), &script); err != nil {
			log.Printf("Ошибка парсинга сценария %d: %v", i+1, err)
			continue
		}

		// Озвучка, если клиент создан
		if saluteClient != nil {
			audioPath, err := saluteClient.Synthesize(script.FullText)
			if err != nil {
				log.Printf("Ошибка озвучки статьи %d: %v", i+1, err)
				continue
			}
			log.Printf("Статья %d озвучена: %s", i+1, audioPath)
		}
		if saluteClient != nil {
			audioPath, err := saluteClient.Synthesize(script.FullText)
			if err != nil {
				log.Printf("Ошибка озвучки статьи %d: %v", i+1, err)
				continue
			}
			log.Printf("Статья %d озвучена: %s", i+1, audioPath)

			// Сборка видео
			videoOutput := filepath.Join("output", fmt.Sprintf("video_%d.mp4", i+1))
			os.MkdirAll("output", 0755)
			if err := compositor.ComposeVertical(compositor.ComposeParams{
				AudioPath:  audioPath,
				Subtitles:  script.Subtitles,
				OutputPath: videoOutput,
			}); err != nil {
				log.Printf("Ошибка сборки видео для статьи %d: %v", i+1, err)
				continue
			}
			log.Printf("Видео для статьи %d собрано: %s", i+1, videoOutput)
		}
	}
}
