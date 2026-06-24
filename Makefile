# ChillCheck developer tasks. Run `make help` for the list.
# Run all targets from the repo root — paths here are relative to it, so
# `make` from a subdirectory fails with "No rule to make target".
PG := backend/db/rootless-postgres.sh

.PHONY: help pg-up pg-dsn pg-down pg-nuke test test-integration

help: ## Show available targets
	@grep -E '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  %-18s %s\n", $$1, $$2}'

pg-up: ## Start the rootless Postgres (downloads on first run) and load the schema
	@$(PG) start

pg-dsn: ## Print the TEST_DATABASE_URL for the rootless Postgres
	@$(PG) dsn

pg-down: ## Stop the rootless Postgres (keeps data for reuse)
	@$(PG) stop

pg-nuke: ## Stop and delete the rootless Postgres entirely
	@$(PG) nuke

test: ## Run the backend unit tests (no database needed)
	cd backend && go test ./...

test-integration: pg-up ## Start the rootless Postgres, then run all backend tests (incl. DB-gated)
	cd backend && TEST_DATABASE_URL="$$(db/rootless-postgres.sh dsn)" go test -count=1 ./...
