# processosplatdistr

Plataforma de Gestão do Ciclo de Vida do Colaborador: um binário Go que serve a
API, o BFF OIDC e o SPA React embutido. Requisitos em [`docs/prd.md`](docs/prd.md)
e arquitetura em [`docs/sdd.md`](docs/sdd.md).

## Desenvolvimento

Requer Go 1.24 (`GOTOOLCHAIN=local`), Node 22, pnpm 10 e MySQL 8 (>= 8.0.16).

```bash
make mysql                       # MySQL local em 127.0.0.1:3306 (app/app); instala na 1ª vez
make build                       # SPA em internal/web/dist + bin/server
cp .env.example .env             # preencha SESSION_ENC_KEY (head -c 32 /dev/urandom | base64)
set -a && . ./.env && set +a
MYSQL_PWD=app mysql --no-defaults --protocol=TCP -h127.0.0.1 -uapp -e 'CREATE DATABASE IF NOT EXISTS app_dev'
bin/server config check          # valida a configuração, com as travas de produção
bin/server serve                 # aplica as migrations e sobe em http://localhost:8080 (/healthz, /readyz)
bin/server migrate status        # situação de cada migration (também: migrate up, migrate force N)
make test                        # go vet + go test + testes do frontend (vitest)
make test-int                    # go test com TEST_MYSQL_DSN: um banco t_<pacote>_<pid> por pacote
make e2e                         # Playwright (Chromium de /opt/pw-browsers) contra bin/server e o banco app_e2e
make lint                        # vet, staticcheck, govulncheck, deps de produção, eslint, tsc
make help                        # demais alvos
```

O `make lint` (e o `make check`) falha se o govulncheck não alcançar
`vuln.go.dev`. Em rede restrita, `make lint VULNCHECK_OFFLINE_OK=1` troca só essa
falha por um aviso, e a verificação de vulnerabilidades fica de fora.

O frontend fica em `web/` (Vite + React + Mantine): `pnpm -C web dev` sobe o Vite
em :5173 com proxy de `/api`, `/auth` e `/s` para o server em :8080, e
`pnpm -C web build` gera o SPA em `internal/web/dist`, embutido no binário.

Deploy no Tsuru por Dockerfile: ver §10 do SDD.
