.PHONY: run clean

# Запуск пайплайна
run:
	go run cmd/main.go

# Очистка временных файлов
clean:
	rm -rf audio/ output/ token_cache.json stock/

# Полный сброс и запуск (clean + run)
fresh: clean run