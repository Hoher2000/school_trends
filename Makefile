.PHONY: run clean

# Запуск пайплайна
run:
	go run cmd/main.go

# Очистка временных файлов
clean:
	rm -rf audio/ output/ token_cache.json

# Полный сброс и запуск (clean + run)
fresh: clean run