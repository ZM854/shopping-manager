.PHONY: air dev db-up db-down db-reset

ifneq (,$(wildcard .env))
    include .env
    export
endif

air:
	air

dev: db-up
	air

db-up:
	docker compose up -d

db-down:
	docker compose down

db-reset:
	@PGPASSWORD="$(DB_PASSWORD)" psql -h localhost -U $(DB_USER) -d $(DB_NAME) -f scripts/clear_users.sql