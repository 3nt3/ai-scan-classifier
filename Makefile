REGISTRY := ghcr.io/3nt3
IMAGE := $(REGISTRY)/ai-scan-classifier
SERVER := 3nt3
DEPLOY_DIR := ai-scan-classifier

export DOCKER_BUILDKIT=1

.PHONY: build push deploy all

all: build push deploy

build:
	docker build -t $(IMAGE):latest .

push:
	docker push $(IMAGE):latest

deploy:
	ssh $(SERVER) "cd $(DEPLOY_DIR) && docker compose pull && docker compose up -d"

logs:
	ssh $(SERVER) "cd $(DEPLOY_DIR) && docker compose logs -f"
