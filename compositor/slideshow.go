package compositor

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func CreateSlideshow(imagePaths []string, outputPath string) error {
	var valid []string
	for _, p := range imagePaths {
		if isImageFile(p) {
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

	cmd := exec.Command("ffmpeg", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("FFmpeg слайдшоу ошибка: %w\nВывод: %s", err, string(out))
	}
	return nil
}

func isImageFile(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if len(data) < 4 {
		return false
	}
	if bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}) {
		return true
	}
	if bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4E, 0x47}) {
		return true
	}
	if bytes.HasPrefix(data, []byte{0x52, 0x49, 0x46, 0x46}) && bytes.Contains(data[:12], []byte{0x57, 0x45, 0x42, 0x50}) {
		return true
	}
	return false
}
