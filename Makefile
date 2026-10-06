# Shared Compose commands for Linux/WSL; services can also be started individually.
COMPOSE = docker compose -f compose.yaml
DEV_COMPOSE = $(COMPOSE) -f compose.dev.yaml
MODE ?= dev
ACTIVE_COMPOSE = $(if $(filter built,$(MODE)),$(COMPOSE),$(DEV_COMPOSE))

.PHONY: init dev up down logs status restart migrate-up migrate-version frontend-check backend-check frontend-test frontend-coverage backend-test backend-coverage backend-integration test-db-up

init:
	@if [ -f .env ]; then echo 'Root .env already exists; kept unchanged.'; \
	else umask 077; \
	access_secret=$$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n'); \
	refresh_secret=$$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n'); \
	test $${#access_secret} -eq 64 && test $${#refresh_secret} -eq 64 && \
	sed -e "s/GENERATE_ACCESS_SECRET/$$access_secret/" -e "s/GENERATE_REFRESH_SECRET/$$refresh_secret/" .env.example > .env && \
	echo 'Created root .env with independent random JWT secrets.'; fi

dev:
	$(DEV_COMPOSE) up --build -d --wait --wait-timeout 300

up:
	$(COMPOSE) up --build -d --wait --wait-timeout 300

down:
	$(ACTIVE_COMPOSE) down --remove-orphans

logs:
	$(ACTIVE_COMPOSE) logs -f --tail 100 $(SERVICE)

status:
	$(ACTIVE_COMPOSE) ps

restart:
	@test -n "$(SERVICE)" || (echo 'Use SERVICE=frontend/backend/postgres/mailpit'; exit 1)
	$(ACTIVE_COMPOSE) restart $(SERVICE)

migrate-up:
	$(ACTIVE_COMPOSE) run --rm migrate

migrate-version:
	$(ACTIVE_COMPOSE) run --rm migrate 'exec migrate -path /migrations -database "postgres://postgres:5432/$$PGDATABASE?sslmode=disable" version'

frontend-check:
	$(DEV_COMPOSE) run --build --rm --no-deps frontend sh -ec 'npm ci --no-audit --no-fund && npm run lint && npm run build'

backend-check:
	$(DEV_COMPOSE) run --build --rm --no-deps backend sh -ec 'go mod verify && go test ./... && go vet ./... && go build -o /tmp/api-check ./cmd/api'

frontend-test:
	npm --prefix frontend run test

frontend-coverage:
	npm --prefix frontend run test:coverage

backend-test:
	cd backend && GOMODCACHE="$(CURDIR)/backend/.tools/go/pkg/mod" GOCACHE="$(CURDIR)/backend/.cache/go-build" go test ./...

backend-coverage:
	mkdir -p backend/.cache
	cd backend && GOMODCACHE="$(CURDIR)/backend/.tools/go/pkg/mod" GOCACHE="$(CURDIR)/backend/.cache/go-build" go test ./... -coverprofile=.cache/coverage.out

backend-integration:
	docker compose -f compose.test.yaml run --build --rm backend-test

test-db-up:
	docker compose -f compose.test.yaml up -d --wait --wait-timeout 90 postgres-test
