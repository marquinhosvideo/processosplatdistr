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
make test                        # go vet + go test + testes do frontend
make lint                        # vet, staticcheck, govulncheck, deps de produção, eslint, tsc
make help                        # demais alvos
```

Deploy no Tsuru por Dockerfile: ver §10 do SDD.
