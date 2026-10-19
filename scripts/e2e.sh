#!/usr/bin/env bash
# E2E (SDD §9.2): compila o binário de produção com o SPA embutido e roda o
# Playwright contra ele. O webServer do web/playwright.config.ts sobe o server.
# Argumentos extras vão para o "playwright test" (ex.: e2e/smoke.spec.ts).
set -euo pipefail

cd "$(dirname "$0")/.."

# Neste ambiente o Node 22 fica fora do PATH padrão.
if [ -d /opt/node22/bin ]; then
  export PATH="/opt/node22/bin:$PATH"
fi
export GOTOOLCHAIN=local
export PLAYWRIGHT_BROWSERS_PATH="${PLAYWRIGHT_BROWSERS_PATH:-/opt/pw-browsers}"
export PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1

make build

# O server do E2E usa o banco app_e2e (web/playwright.config.ts) e aplica as
# migrations na subida. Com E2E_DATABASE_DSN, o banco é por conta de quem
# chama; senão, o MySQL local sobe (ou é reaproveitado) e o banco é recriado.
if [ -z "${E2E_DATABASE_DSN:-}" ]; then
  scripts/mysql-local.sh start
  if [ -x bin/devseed ]; then
    bin/devseed -dsn "${TEST_MYSQL_DSN:-app:app@tcp(127.0.0.1:3306)/}" -db app_e2e -reset -profile e2e
  else
    # Até o devseed existir, o E2E começa de um banco vazio.
    MYSQL_PWD=app mysql --no-defaults --protocol=TCP -h127.0.0.1 -P3306 -uapp \
      -e 'DROP DATABASE IF EXISTS app_e2e; CREATE DATABASE app_e2e'
  fi
fi

# Nunca "playwright install": o Chromium já está em PLAYWRIGHT_BROWSERS_PATH.
exec pnpm -C web exec playwright test "$@"
