package collector

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DownloadWithYtDlp запускает yt-dlp с переданными аргументами и возвращает путь к скачанному файлу.
// args – дополнительные аргументы после "yt-dlp" (например, "ytsearch1:minecraft gameplay")
func DownloadWithYtDlp(args ...string) (string, error) {
	// Убедимся, что папка backgrounds существует
	os.MkdirAll("backgrounds", 0755)

	// Стандартные аргументы: сохранять в папку backgrounds, не перезаписывать, имя по шаблону
	baseArgs := []string{
		"--output", "backgrounds/%(title)s-%(id)s.%(ext)s",
		"--no-playlist",
		"--max-filesize", "50m", // ограничим размер, чтобы не качать слишком тяжёлые видео
	}
	allArgs := append(baseArgs, args...)

	cmd := exec.Command("yt-dlp", allArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("yt-dlp ошибка: %w\nВывод: %s", err, string(output))
	}

	// Ищем скачанный файл по выводу: обычно последняя строка содержит "Destination: <путь>"
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "[download] Destination: ") {
			destPath := strings.TrimPrefix(line, "[download] Destination: ")
			// Проверяем существование файла
			if _, err := os.Stat(destPath); err == nil {
				return destPath, nil
			}
		}
	}
	// Если Destination не найден, попробуем найти любой свежий файл в backgrounds
	entries, err := os.ReadDir("backgrounds")
	if err != nil {
		return "", fmt.Errorf("не удалось прочитать папку backgrounds: %w", err)
	}
	for i := len(entries) - 1; i >= 0; i-- {
		info, err := entries[i].Info()
		if err == nil && !info.IsDir() {
			return filepath.Join("backgrounds", entries[i].Name()), nil
		}
	}
	return "", fmt.Errorf("не удалось определить скачанный файл")
}
