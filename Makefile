export UID := $(shell id -u)
export GID := $(shell id -g)

COMPOSE := docker compose
OUTILS := $(COMPOSE) run --rm outils
GO_PAQUETS := ./...

.PHONY: help up down logs build test test-race test-integration test-coverage lint fmt tidy simulate migrate

help: ## Affiche cette aide
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

up: ## Démarre Postgres, Redis et le serveur (http://localhost:8080)
	$(COMPOSE) up -d --build --wait serveur

down: ## Arrête la stack
	$(COMPOSE) down

logs: ## Suit les logs du serveur
	$(COMPOSE) logs -f serveur

build: ## Construit l'image Docker de production
	docker build -t coinchenet:local .

test: ## Tests unitaires (sans base de données)
	$(OUTILS) go test $(GO_PAQUETS)

test-race: ## Tests unitaires avec le détecteur de concurrence
	$(OUTILS) go test -race $(GO_PAQUETS)

test-integration: ## Tests d'intégration contre Postgres et Redis
	$(COMPOSE) up -d --wait postgres redis
	$(OUTILS) go test -tags integration -count=1 -p 1 $(GO_PAQUETS)

test-coverage: ## Couverture des tests unitaires
	$(OUTILS) sh -c 'go test -coverprofile=coverage.out $(GO_PAQUETS) && go tool cover -func=coverage.out | tail -1'

lint: ## Analyse statique (golangci-lint)
	$(COMPOSE) run --rm lint golangci-lint run $(GO_PAQUETS)

fmt: ## Formate le code
	$(COMPOSE) run --rm lint golangci-lint fmt $(GO_PAQUETS)

tidy: ## Met à jour go.mod / go.sum
	$(OUTILS) go mod tidy

simulate: ## Fait jouer des parties entre bots (N=1000 par défaut)
	$(OUTILS) go run ./cmd/coinchenet simuler -parties $${N:-1000}

migrate: ## Applique les migrations sur la base de la stack
	$(COMPOSE) run --rm serveur migrer
