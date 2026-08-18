PLANT_DIR := doc/diagram
PLANT_OUT_DIR := $(PLANT_DIR)/generated
TOOLS_IMAGE := connkeep-tools:local
TOOLS_DOCKERFILE := docker/tools.Dockerfile
TOOLS_CONTAINER := connkeep-tools
DOCKER_NETWORK := connkeep
GOMOD_CACHE_VOLUME := connkeep-gomod
GOBUILD_CACHE_VOLUME := connkeep-gobuild
GOLANGCI_CACHE_VOLUME := connkeep-golangci
PROJECT_DIR ?= $(or $(PWD),$(CURDIR),$(shell pwd))

ifeq ($(strip $(PROJECT_DIR)),)
$(error Cannot determine project directory. Run make from repository root)
endif

.PHONY: generate generate-svg generate-png generate-compare clean ensure-network ensure-tools-image ensure-tools-container stop-tools-container test lint build-examples prepush

ensure-network:
	@docker network inspect $(DOCKER_NETWORK) >/dev/null 2>&1 || docker network create $(DOCKER_NETWORK) >/dev/null

ensure-tools-image:
	@docker build -t $(TOOLS_IMAGE) -f $(TOOLS_DOCKERFILE) . >/dev/null

ensure-tools-container: ensure-network ensure-tools-image
	@image_id=$$(docker image inspect -f '{{.Id}}' $(TOOLS_IMAGE)); \
	container_image_id=$$(docker inspect -f '{{.Image}}' $(TOOLS_CONTAINER) 2>/dev/null || true); \
	if [ -n "$$container_image_id" ] && [ "$$container_image_id" != "$$image_id" ]; then \
		docker rm -f $(TOOLS_CONTAINER) >/dev/null 2>&1 || true; \
	fi; \
	docker container inspect $(TOOLS_CONTAINER) >/dev/null 2>&1 || docker create --name $(TOOLS_CONTAINER) --network $(DOCKER_NETWORK) -v "$(PROJECT_DIR):/workspace" -v $(GOMOD_CACHE_VOLUME):/go/pkg/mod -v $(GOBUILD_CACHE_VOLUME):/root/.cache/go-build -v $(GOLANGCI_CACHE_VOLUME):/root/.cache/golangci-lint -w /workspace $(TOOLS_IMAGE) sleep infinity >/dev/null; \
	docker inspect -f '{{.State.Running}}' $(TOOLS_CONTAINER) 2>/dev/null | grep -q true || docker start $(TOOLS_CONTAINER) >/dev/null

stop-tools-container:
	@docker stop $(TOOLS_CONTAINER) >/dev/null 2>&1 || true

# Генерация SVG
-generate-svg:
	@docker exec $(TOOLS_CONTAINER) sh -c "set -eu; find '$(PLANT_DIR)' -type f -name '*.plant' | while IFS= read -r file; do rel_path=\$${file#$(PLANT_DIR)/}; rel_dir=\$$(dirname \"\$$rel_path\"); mkdir -p '$(PLANT_OUT_DIR)'/svg/\"\$$rel_dir\"; java -jar /opt/plantuml.jar -tsvg -output '$(PLANT_OUT_DIR)'/svg/\"\$$rel_dir\" \"\$$file\"; done"

generate-svg: ensure-tools-container -generate-svg stop-tools-container


# Генерация PNG (опционально)
-generate-png:
	@docker exec $(TOOLS_CONTAINER) sh -c "set -eu; find '$(PLANT_DIR)' -type f -name '*.plant' | while IFS= read -r file; do rel_path=\$${file#$(PLANT_DIR)/}; rel_dir=\$$(dirname \"\$$rel_path\"); mkdir -p '$(PLANT_OUT_DIR)'/png/\"\$$rel_dir\"; java -jar /opt/plantuml.jar -tpng -output '$(PLANT_OUT_DIR)'/png/\"\$$rel_dir\" \"\$$file\"; done"

generate-png: ensure-tools-container -generate-png stop-tools-container


generate: ensure-tools-container -generate-svg -generate-png

# Сравнение сгенерированных диаграмм с HEAD
-generate-compare:
	@docker exec $(TOOLS_CONTAINER) bash scripts/generate-compare.sh /workspace "$(PLANT_DIR)" "$(PLANT_OUT_DIR)"

generate-compare: ensure-tools-container -generate-compare stop-tools-container


# Запуск юнит-тестов
-test:
	@docker exec $(TOOLS_CONTAINER) sh -c "go test ./..."

test: ensure-tools-container -test stop-tools-container

# Запуск lint
-lint:
	@docker exec $(TOOLS_CONTAINER) sh -c "golangci-lint run ./... --timeout 5m"

lint: ensure-tools-container -lint stop-tools-container

# Очистка сгенерированных файлов
-clean:
	@docker exec $(TOOLS_CONTAINER) sh -c "rm -rf '$(PLANT_OUT_DIR)'"

clean: ensure-tools-container -clean stop-tools-container

# Проверка компиляции примеров
-build-examples:
	docker exec $(TOOLS_CONTAINER) bash scripts/build-examples.sh /workspace

build-examples: ensure-tools-container -build-examples stop-tools-container

prepush: ensure-tools-container -generate-svg -generate-png -generate-compare -lint -test -build-examples stop-tools-container


