# processosplatdistr

Plataforma de Gestão do Ciclo de Vida do Colaborador: um binário Go que serve a
API, o BFF OIDC e o SPA React embutido. Requisitos em [`docs/prd.md`](docs/prd.md)
e arquitetura em [`docs/sdd.md`](docs/sdd.md).

## Desenvolvimento

Requer Go 1.24 (`GOTOOLCHAIN=local`), Node 22 e pnpm 10.

```bash
make build                       # SPA em internal/web/dist + bin/server
cp .env.example .env             # preencha SESSION_ENC_KEY (head -c 32 /dev/urandom | base64)
set -a && . ./.env && set +a
bin/server config check          # valida a configuração, com as travas de produção
bin/server serve                 # http://localhost:8080 (/healthz, SPA em qualquer outra rota)
make test                        # go vet + go test + testes do frontend (vitest)
make e2e                         # Playwright (Chromium de /opt/pw-browsers) contra bin/server
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
