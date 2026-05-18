package compositor

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type ComposeParams struct {
	AudioPath  string
	Subtitles  []string
	OutputPath string
}

func ComposeVertical(params ComposeParams, bgVideoPath string) error {
	if _, err := os.Stat(params.AudioPath); err != nil {
		return fmt.Errorf("аудиофайл не найден: %w", err)
	}

	audioDur, err := GetAudioDuration(params.AudioPath)
	if err != nil {
		return fmt.Errorf("не удалось определить длительность аудио: %w", err)
	}

	srtPath := params.OutputPath + ".srt"
	if err := createSRT(params.Subtitles, audioDur, srtPath); err != nil {
		return fmt.Errorf("создать SRT: %w", err)
	}
	defer os.Remove(srtPath)
	// Путь к папке со шрифтами
	fontsDir := filepath.Join(projectRoot(), "fonts")
	args := []string{}

	if bgVideoPath != "" {
		args = append(args,
			"-stream_loop", "-1",
			"-i", bgVideoPath,
			"-i", params.AudioPath,
			"-filter_complex", fmt.Sprintf(
				"scale=1080:1920:force_original_aspect_ratio=decrease,pad=1080:1920:(ow-iw)/2:(oh-ih)/2,setsar=1,pad=ceil(iw/2)*2:ceil(ih/2)*2,subtitles=%s:fontsdir=%s:force_style='Fontname=Rubik Moonrocks,Fontsize=24,PrimaryColour=&H00FFFFFF,OutlineColour=&H00000000,Outline=1,Shadow=1'",
				srtPath, fontsDir,
			),
			"-map", "0:v",
			"-map", "1:a",
			"-t", fmt.Sprintf("%.3f", audioDur.Seconds()),
		)
	} else {
		args = append(args,
			"-f", "lavfi",
			"-i", fmt.Sprintf("color=c=black:s=1080x1920:d=%.3f", audioDur.Seconds()),
			"-i", params.AudioPath,
			"-filter_complex", fmt.Sprintf("subtitles=%s:fontsdir=%s:force_style='Fontname=Rubik Moonrocks,Fontsize=24,PrimaryColour=&H00FFFFFF,OutlineColour=&H00000000,Outline=1,Shadow=1'",
				srtPath, fontsDir),
			"-map", "0:v",
			"-map", "1:a",
		)
	}

	args = append(args,
		"-c:v", "libx264",
		"-preset", "fast",
		"-y", params.OutputPath,
	)

	log.Printf("FFmpeg args: %v", args)
	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("FFmpeg ошибка: %w\nВывод: %s", err, string(output))
	}
	return nil
}

// остальные функции (getAudioDuration, createSRT, formatSRTTime) без изменений
func GetAudioDuration(path string) (time.Duration, error) {
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

func createSRT(phrases []string, totalDuration time.Duration, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if len(phrases) == 0 {
		return fmt.Errorf("пустой список субтитров")
	}
	segment := totalDuration / time.Duration(len(phrases))
	for i, phrase := range phrases {
		start := segment * time.Duration(i)
		end := start + segment
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

func formatSRTTime(d time.Duration) string {
	ms := d.Milliseconds() % 1000
	sec := int(d.Seconds()) % 60
	min := int(d.Minutes()) % 60
	hr := int(d.Hours())
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hr, min, sec, ms)
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
