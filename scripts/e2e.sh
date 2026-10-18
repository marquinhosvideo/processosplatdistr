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

# A massa do E2E chega com o devseed; até lá os specs não dependem do banco.
if [ -x bin/devseed ]; then
  bin/devseed -dsn "${TEST_MYSQL_DSN:-app:app@tcp(127.0.0.1:3306)/}" -db app_e2e -reset -profile e2e
fi

# Nunca "playwright install": o Chromium já está em PLAYWRIGHT_BROWSERS_PATH.
exec pnpm -C web exec playwright test "$@"
