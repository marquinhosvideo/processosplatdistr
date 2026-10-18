# Alvos de desenvolvimento, teste e build (SDD §9.2). Requer Go 1.24 e, para o
# frontend, Node 22 + pnpm 10. Alvos de marcos futuros avisam e saem sem erro
# até o script correspondente existir.

SHELL := /bin/bash
.DEFAULT_GOAL := help

export GOTOOLCHAIN := local
export PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD := 1
export PLAYWRIGHT_BROWSERS_PATH ?= /opt/pw-browsers

# Neste ambiente o Node 22 fica fora do PATH padrão.
NODE_BIN ?= /opt/node22/bin
ifneq ($(wildcard $(NODE_BIN)/pnpm),)
export PATH := $(NODE_BIN):$(PATH)
endif

PNPM           ?= pnpm
VERSION        ?= $(shell git describe --always --dirty --abbrev=12 2>/dev/null || echo dev)
LDFLAGS        := -s -w -X github.com/marquinhosvideo/processosplatdistr/internal/buildinfo.Version=$(VERSION)
TEST_MYSQL_DSN ?= app:app@tcp(127.0.0.1:3306)/
MYSQL_DEV_DB   ?= app_dev

# Binários auxiliares (devidp, devseed) entram no build quando existirem.
AUX_CMDS := $(filter-out server,$(notdir $(wildcard cmd/*)))

.PHONY: help build web-deps web-build go-build test test-go test-web test-int \
	lint vet staticcheck vulncheck check-prod-deps lint-web check \
	mysql seed dev e2e docker clean

help: ## lista os alvos
	@grep -hE '^[a-z0-9-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-16s %s\n", $$1, $$2}'

## --- build ---

build: web-build go-build ## SPA (internal/web/dist) + bin/server e auxiliares

web/node_modules/.modules.yaml: web/package.json web/pnpm-lock.yaml
	$(PNPM) -C web install --frozen-lockfile
	@touch $@

web-deps: web/node_modules/.modules.yaml ## instala as dependências do frontend

web-build: web-deps ## compila o SPA para internal/web/dist
	$(PNPM) -C web build

go-build: ## compila bin/server (com o SPA que estiver em internal/web/dist)
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/server ./cmd/server
	@for c in $(AUX_CMDS); do echo "go build -o bin/$$c ./cmd/$$c"; CGO_ENABLED=0 go build -trimpath -o bin/$$c ./cmd/$$c || exit 1; done

docker: ## imagem de produção (precisa de dockerd)
	docker build --build-arg VERSION=$(VERSION) -t processos-platdistr:$(VERSION) .

## --- testes ---

test: vet test-go test-web ## go vet + go test + testes do frontend

test-go:
	go test ./...

test-web: web-deps
	$(PNPM) -C web test --run

test-int: ## testes de integração com MySQL (make mysql antes)
	TEST_MYSQL_DSN='$(TEST_MYSQL_DSN)' go test -count=1 -p 4 ./...

e2e: ## Playwright contra o binário de produção
	@if [ -x scripts/e2e.sh ]; then scripts/e2e.sh; \
	else echo "AVISO: scripts/e2e.sh ainda não existe; E2E ainda não implementado" >&2; fi

check: lint test test-int ## tudo o que roda antes de commitar

## --- análise estática ---

lint: vet staticcheck vulncheck check-prod-deps lint-web ## vet, staticcheck, govulncheck, deps de produção, eslint e tsc

vet:
	go vet ./...

staticcheck:
	go tool staticcheck ./...

# Sem acesso ao banco de vulnerabilidades (rede restrita), avisa e segue;
# qualquer outra falha, inclusive vulnerabilidade encontrada, interrompe.
vulncheck:
	@out=$$(go tool govulncheck ./... 2>&1); status=$$?; \
	if [ $$status -ne 0 ] && grep -q 'fetching vulnerabilities' <<<"$$out"; then \
		echo "AVISO: govulncheck sem acesso a vuln.go.dev; verificação pulada" >&2; \
	else \
		[ -n "$$out" ] && echo "$$out"; exit $$status; \
	fi

# O IdP mock e a massa de dev nunca podem entrar no binário de produção.
check-prod-deps:
	@if go list -deps ./cmd/server | grep -E '/internal/(devidp|devseed)(/|$$)'; then \
		echo "ERRO: cmd/server depende de pacote de desenvolvimento" >&2; exit 1; \
	fi
	@echo "check-prod-deps: ok"

lint-web: web-deps
	$(PNPM) -C web lint
	$(PNPM) -C web typecheck

## --- ambiente local ---

mysql: ## sobe o MySQL 8 local (scripts/mysql-local.sh start)
	@if [ -x scripts/mysql-local.sh ]; then scripts/mysql-local.sh start; \
	else echo "AVISO: scripts/mysql-local.sh ainda não existe; MySQL local ainda não implementado" >&2; fi

seed: ## recria o banco de dev com a massa do devseed
	@if [ -d cmd/devseed ]; then go run ./cmd/devseed -dsn '$(TEST_MYSQL_DSN)' -db $(MYSQL_DEV_DB) -reset -profile dev; \
	else echo "AVISO: cmd/devseed ainda não existe; massa de dev ainda não implementada" >&2; fi

dev: ## devidp + server + Vite com proxy
	@if [ -x scripts/dev.sh ]; then scripts/dev.sh; \
	else echo "AVISO: scripts/dev.sh ainda não existe; ambiente de dev ainda não implementado (use make build e bin/server serve)" >&2; fi

clean: ## remove bin/ e o build do SPA (preserva o robots.txt versionado e o .tmp/)
	rm -rf bin
	find internal/web/dist -mindepth 1 -not -name robots.txt -delete
