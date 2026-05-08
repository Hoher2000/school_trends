package compositor

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

// ComposeParams описывает входные данные для сборки видео.
type ComposeParams struct {
	AudioPath  string
	Subtitles  []string
	OutputPath string
}

// ComposeVertical создаёт вертикальное видео 1080×1920 с аудио и субтитрами,
// синхронизированными с реальной длительностью аудио.
func ComposeVertical(params ComposeParams) error {
	// 1. Узнаём точную длительность аудио
	duration, err := getAudioDuration(params.AudioPath)
	if err != nil {
		return fmt.Errorf("узнать длительность аудио: %w", err)
	}

	// 2. Создаём временный SRT-файл с равномерным распределением фраз
	srtPath := params.OutputPath + ".srt"
	if err := createSRT(params.Subtitles, duration, srtPath); err != nil {
		return fmt.Errorf("создать SRT: %w", err)
	}
	defer os.Remove(srtPath)

	// 3. Генерируем чёрный фон и собираем видео
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi",
		"-i", fmt.Sprintf("color=c=black:s=1080x1920:d=%.3f", duration.Seconds()),
		"-i", params.AudioPath,
		"-filter_complex", fmt.Sprintf("subtitles=%s:force_style='Fontsize=24,Alignment=2'", srtPath),
		"-map", "0:v",
		"-map", "1:a",
		"-c:v", "libx264",
		"-preset", "fast",
		"-shortest",
		"-y", params.OutputPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("FFmpeg ошибка: %w\nВывод: %s", err, string(output))
	}
	return nil
}

// createSRT записывает субтитры, равномерно распределяя их по общей длительности.
func createSRT(phrases []string, totalDuration time.Duration, path string) error {
	if len(phrases) == 0 {
		return fmt.Errorf("пустой список субтитров")
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	segment := totalDuration / time.Duration(len(phrases))
	for i, phrase := range phrases {
		start := segment * time.Duration(i)
		end := start + segment
		// последнюю фразу фиксируем точно по концу аудио
		if i == len(phrases)-1 {
			end = totalDuration
		}
		_, err := fmt.Fprintf(f, "%d\n%s --> %s\n%s\n\n",
			i+1,
			formatSRTTime(start),
			formatSRTTime(end),
			phrase,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// formatSRTTime форматирует время для SRT (hh:mm:ss,ms)
func formatSRTTime(d time.Duration) string {
	ms := d.Milliseconds() % 1000
	sec := int(d.Seconds()) % 60
	min := int(d.Minutes()) % 60
	hr := int(d.Hours())
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hr, min, sec, ms)
}

// getAudioDuration возвращает длительность аудиофайла через ffprobe.
func getAudioDuration(path string) (time.Duration, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe: %w", err)
	}
	var seconds float64
	_, err = fmt.Sscanf(string(out), "%f", &seconds)
	if err != nil {
		return 0, fmt.Errorf("парсинг длительности: %w", err)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}
