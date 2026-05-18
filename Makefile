.PHONY: run clean build fresh video copy start-openserp stop-openserp

ARCH := $(shell uname -m)
ifeq ($(ARCH), aarch64)
    GOARCH = arm64
else ifeq ($(ARCH), armv7l)
    GOARCH = arm
else
    GOARCH = amd64
endif

BINARY = school_trends_$(GOARCH)
GOOS = linux
GOFLAGS = -ldflags="-s -w"
OUTPUT_DIR ?= $(HOME)/school_trends/output
MAX ?= 3
SDCARD_DIR = /sdcard/school_trends
OPENSERP_CONTAINER = openserp
OPENSERP_PORT = 7000

# Сборка бинарника
build:
	@echo "Сборка для $(GOOS)/$(GOARCH)..."
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(GOFLAGS) -o $(BINARY) ./cmd
	@echo "Готово: $(BINARY)"

# Запуск пайплайна
run:
	./$(BINARY) --output $(OUTPUT_DIR) --max_articles $(MAX)

# Очистка временных файлов
clean:
	rm -rf audio/ output/ stock/ token_cache.json backgrounds/
	rm -f $(BINARY)

# Запуск OpenSERP контейнера (если ещё не запущен)
start-openserp:
	@echo "Проверяю OpenSERP контейнер..."
	@if [ $$(docker ps -q -f name=$(OPENSERP_CONTAINER)) ]; then \
		echo "Контейнер $(OPENSERP_CONTAINER) уже запущен."; \
	elif [ $$(docker ps -aq -f name=$(OPENSERP_CONTAINER)) ]; then \
		echo "Запускаю остановленный контейнер..."; \
		docker start $(OPENSERP_CONTAINER); \
	else \
		echo "Создаю и запускаю новый контейнер..."; \
		docker run -d --name $(OPENSERP_CONTAINER) -p 127.0.0.1:$(OPENSERP_PORT):7000 karust/openserp serve -a 0.0.0.0 -p 7000; \
	fi
	@echo "Ожидаю готовности OpenSERP..."
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
		if curl -s http://127.0.0.1:$(OPENSERP_PORT)/yandex/image?text=test\&limit=1 > /dev/null; then \
			echo "OpenSERP готов."; \
			break; \
		fi; \
		echo "Жду..."; \
		sleep 2; \
	done

# Остановка OpenSERP контейнера
stop-openserp:
	@echo "Останавливаю контейнер $(OPENSERP_CONTAINER)..."
	-docker stop $(OPENSERP_CONTAINER)

# Полный цикл с запуском и остановкой контейнера
fresh: clean build start-openserp run stop-openserp
	@echo "Пайплайн завершён. Видео в $(OUTPUT_DIR)/video/"