package compositor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func CreateSlideshow(imagePaths []string, outputPath string) error {
	var valid []string
	for _, p := range imagePaths {
		if isProbablyImage(p) {
			valid = append(valid, p)
		} else {
			fmt.Printf("Пропущен не-картинка: %s\n", p)
		}
	}
	if len(valid) == 0 {
		return fmt.Errorf("нет подходящих изображений")
	}

	args := []string{"-y"}
	for _, img := range valid {
		args = append(args, "-loop", "1", "-t", "5", "-i", img)
	}

	// Новый фильтр: вписываем с сохранением пропорций и добавляем чёрные поля
	var filterParts []string
	for i := range valid {
		part := fmt.Sprintf(
			"[%d:v]scale=1080:1920:force_original_aspect_ratio=decrease,pad=1080:1920:(ow-iw)/2:(oh-ih)/2,setsar=1,pad=ceil(iw/2)*2:ceil(ih/2)*2[v%d]",
			i, i,
		)
		filterParts = append(filterParts, part)
	}
	filter := strings.Join(filterParts, ";")
	if len(valid) > 0 {
		filter += ";"
	}
	for i := range valid {
		filter += fmt.Sprintf("[v%d]", i)
	}
	filter += fmt.Sprintf("concat=n=%d:v=1:a=0,format=yuv420p[v]", len(valid))

	args = append(args,
		"-filter_complex", filter,
		"-map", "[v]",
		"-t", fmt.Sprintf("%d", len(valid)*5),
		"-c:v", "libx264",
		"-preset", "fast",
		outputPath,
	)

	// Тайм-аут 5 минуты
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	out, err := cmd.CombinedOutput()

	// Всегда проверяем ошибку контекста ПЕРЕД ошибкой FFmpeg
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("слайдшоу прервано по тайм-ауту: %w", ctx.Err())
	}
	if err != nil {
		return fmt.Errorf("FFmpeg слайдшоу ошибка: %w\nВывод: %s", err, string(out))
	}
	return nil
}

// isValidImage проверяет, что файл является изображением (через ffprobe)
func isValidImage(path string) bool {
	cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_type", "-of", "csv=p=0", path)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "video") || strings.Contains(string(out), "image")
}

func isProbablyImage(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 4 {
		return false
	}
	// JPEG
	if bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}) {
		return true
	}
	// PNG
	if bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4E, 0x47}) {
		return true
	}
	// WebP
	if bytes.HasPrefix(data, []byte{0x52, 0x49, 0x46, 0x46}) && len(data) >= 12 && string(data[8:12]) == "WEBP" {
		return true
	}
	// Иначе полная проверка через ffprobe
	return isValidImage(path)
}
