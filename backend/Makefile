.PHONY: \
	air \
	dev \
	db-up \
	db-down \
	db-reset \
	migrate-up \
	migrate-down \
	migrate-version \
	migrate-force \
	create-migration

ifneq (,$(wildcard .env))
    include .env
    export
endif

MIGRATIONS_DIR=./migrations
DB_URL=postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=$(DB_SSLMODE)

air:
	air

dev: db-up migrate-up
	air

db-up:
	docker compose up -d

db-down:
	docker compose down

db-reset:
	@PGPASSWORD="$(DB_PASSWORD)" psql -h localhost -U $(DB_USER) -d $(DB_NAME) -f scripts/clear_users.sql

migrate-up:
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_URL)" up

migrate-down:
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_URL)" down 1

migrate-force:
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_URL)" force $(VERSION)

migrate-version:
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_URL)" version

create-migration:
	migrate create -ext sql -dir $(MIGRATIONS_DIR) -seq $(NAME)