# SDD — Plataforma de Gestão do Ciclo de Vida do Colaborador (MVP)

| Campo | Valor |
|---|---|
| Status | **v1.0 — em revisão pelo PO** |
| Versão | 1.0. Consolida três designs independentes a partir do "pragmático", que venceu, e incorpora as correções dos juízes. |
| Base | `docs/prd.md` v0.1, lido por inteiro, mais as decisões já tomadas pelo usuário |
| Destino | `docs/sdd.md`, commitado no M0 |
| Escopo | Todos os requisitos **M**, mais estes **S**: RF-AUTH-03, RF-REC-08, RF-REC-09 (stub), dados restritos (§4.1), RF-ONB-06, RF-ONB-07, RF-9BOX-07, RF-9BOX-09, RF-TRN-06 (in-app), RF-TRN-07 (CSV e impressão) e RNF-09 a RNF-12 |
| Fora do escopo | Os **C** (RF-REC-11, ONB-08, 9BOX-10, TRN-08), escrita no ERP, e-mail/Slack e PDF gerado no servidor |

Ambiente conferido em modo leitura:
- Ubuntu 24.04, rodando como uid 0.
- Go 1.24.7, Node 22.22.0, pnpm 10.28.0.
- Playwright 1.56.1 global, com `chromium-1194` em `/opt/pw-browsers`.
- MySQL ainda não instalado.
- Remote `github.com/marquinhosvideo/processosplatdistr`, que vira o module path Go.

---

## 1. Arquitetura geral e estrutura de diretórios

### 1.1 Visão geral

```
Browser ── SPA React/Mantine (servida pelo binário)
  │ cookie __Host-pp_sid (opaco, HttpOnly, Secure, SameSite=Lax)
  ▼
Tsuru router ─► server (Go, 1 binário, N unidades)
                ├─ /auth/*          BFF OIDC (Code+PKCE) ───────────────► IdP (Keycloak/Azure AD)
                ├─ /api/v1/*        JSON (sessão → gate → papel → CSRF → handler) ─► MySQL 8 (DBaaS)
                ├─ /s/*             página pública do onboarding (token no fragmento #)
                ├─ /healthz /readyz /metrics
                ├─ /*               SPA embutida (go:embed), fallback index.html
                ├─ jobs.Runner      goroutine; 1 execução por slot via UNIQUE(job_name, slot_at)
                └─ adapters         erp.Source(CSV) · gupy.Client(Stub, flag) · files.Store(MySQL|S3) · files.Scanner(noop|clamd)
```

**Monólito modular em duas camadas: handler HTTP e módulo.** O módulo concentra regra de negócio e SQL escrito à mão. Para cerca de 50 usuários, um time e um app Tsuru, camadas de repositório, DI ou eventos não se pagam.
- A lógica pura fica em funções sem I/O, testáveis sem banco: máquinas de estado, políticas de acesso, datas e quadrantes.
- Interfaces existem só nas fronteiras externas: `files.Store`, `files.Scanner`, `erp.Source` e `gupy.Client`.
- O relógio é injetado (`now func() time.Time`).

### 1.2 Estrutura do repositório

```
cmd/
  server/main.go        # subcomandos: serve | migrate [up|status|force N] | jobs run <nome> | config check | version
  devidp/main.go        # IdP OIDC mock (dev/test/E2E); NUNCA importado por cmd/server
  devseed/main.go       # -dsn -db <nome> -reset -profile dev|e2e : recria banco, migra, insere massa
internal/
  app/                  # composition root: config → db → migrate → bootstrap → módulos → server → jobs
  buildinfo/            # Version (ldflags)
  config/               # Load() de env, Validate() com travas de produção, tipo Secret (redigido em logs)
  db/                   # Open (DSN/URL do DBaaS, TLS, parâmetros), InTx, interface Q, helpers IsDuplicate/IsFKViolation/IsLockNowait
  db/migrate/           # runner próprio (GET_LOCK, checksum, dirty, force)
  db/dbtest/            # Main(m) cria banco por pacote; Reset(t); fixtures builders
  httpx/                # JSON in/out, problem+json, paginação, validação V, middlewares, clientip, ratelimit
  seal/                 # AES-256-GCM com SESSION_ENC_KEY (+ _PREVIOUS); RandomToken; SHA256
  auth/                 # oidc.go, session.go, recheck.go, roles.go, returnto.go, principal.go, handlers.go, csrf.go, pages/ (negado, saiu)
  auth/authtest/        # Session(t, db, persona) → cookie para testes HTTP
  devidp/               # implementação do IdP mock (usada por cmd/devidp e por testes in-process)
  devseed/data/         # massa embutida do devseed (só cmd/devseed importa)
  audit/                # Record (snapshots com tag audit:"plain|redacted"), Access (fail-closed), handlers
  settings/             # app_settings + defaults em Go + validação por chave + cache 30 s; flags "feature.*"
  notify/               # notificações in-app (Create com dedupe, List, Read)
  org/                  # org units, times, pessoas, cargos, usuários (admin), ManagedTeams, gestor efetivo
  org/erp/              # Snapshot, Source, CSVSource, Diff, Apply, lotes (preview/apply/discard/reprocess)
  catalog/              # skills, competências, cargos (CRUD admin)
  files/                # Store (mysql, s3), Scanner (noop, clamd), sniff.go, upload.go (MultipartReader), download.go
  recruiting/           # candidatos, vagas, dedup, busca, funil, timeline, entrevistas, restritos, LGPD, anonymize.go, import CSV
  recruiting/gupy/      # Client, Stub (fixtures embutidas), Sync
  quarters/             # quarters, vigente/próximo, ensure
  training/             # pleitos, beneficiados, transições, evidências, lote, orçamento, painel, CSV
  ninebox/              # ciclo do quarter, participantes, registros, eventos, comentários, matriz, histórico, cobertura, movimentação
  onboarding/           # catálogo + revisões, planos, checklist, links, página pública /s, export .md, painel
  home/                 # GET /api/v1/home (pendências por papel)
  insights/             # métricas de sucesso do PRD §9
  jobs/                 # Runner, Schedule (Hourly/Daily/Weekly), registro de jobs
  server/               # routes.go (tabela ÚNICA de rotas + RoleSet), server.go (http.Server, cadeia de middlewares)
  web/                  # embed.go (//go:embed all:dist), spa.go; dist/robots.txt versionado
  web/share/            # share.html, share.css, share.js (página pública, embutidos)
migrations/             # 0001_core.sql … 0007_onboarding.sql + embed.go (//go:embed *.sql)
web/                    # Vite + React + TS; web/e2e (Playwright); web/public/robots.txt
dev/                    # personas.json, clients.json, sample-erp.csv, sample-candidates.csv
scripts/                # mysql-local.sh, dev.sh, e2e.sh
docs/                   # prd.md, sdd.md, lgpd.md (registro de tratamento), runbook.md
Dockerfile .dockerignore tsuru.yaml Procfile Makefile .env.example .gitignore
```

**Arquivos de cada módulo:**

| Arquivo | Conteúdo |
|---|---|
| `handlers.go` | Decodifica, valida formato e chama o módulo |
| `module.go` | `type Module struct{ db *sql.DB; now func() time.Time; ... }`, com um método por caso de uso. Cada método abre a tx, carrega os fatos, aplica a política e grava dados, evento de domínio e auditoria. |
| `model.go` | Tipos, enums e FSM (funções puras) |
| `policy.go` | Autorização por objeto (funções puras) |
| `sql.go` | Queries |
| `dto.go` | Structs de resposta por caso de uso. O dado restrito só existe no DTO restrito. |

**Dependências entre módulos** (sem ciclos):
- Todos os módulos podem usar `db`, `httpx`, `auth`, `audit`, `settings`, `notify`, `files` e `org`.
- `recruiting` usa `onboarding` através da interface `onboarding.DraftCreator`, injetada no `app`, para o RF-REC-08.
- `home` e `insights` só leem.

**Cadeia de middlewares** (em `server.go`):

```
recover → requestID (16 hex, X-Request-Id) → securityHeaders → clientIP (TRUSTED_PROXY_CIDRS) → accessLog (slog, r.Pattern)
  /api/v1/*:  noStore → csrf (métodos não seguros) → session.Require (401) → RoleSet da rota (403) → MaxBytes/JSON (413/415) → handler
  /auth/*:    noStore → ratelimit(login/callback) → handler        /s/api/*: noStore → ratelimit → JSON → handler
```

**Assinatura de handler:** `type HandlerFunc func(w http.ResponseWriter, r *http.Request, p auth.Principal) error`. O erro devolvido é renderizado num único ponto (`httpx.WriteError`), sempre como problem+json.

```go
type Principal struct {
    UserID    int64
    PersonID  int64        // 0 = sem vínculo com people
    OrgUnitID int64        // gerência da pessoa ou DEFAULT_ORG_UNIT
    Roles     auth.RoleSet // Manager | Head | Admin (Head ⇒ Manager)
    Name, Email string
    SessionHash [32]byte
}
```

---

## 2. Bibliotecas

### 2.1 Backend

`go.mod` declara `go 1.24.0`. Todo build roda com `GOTOOLCHAIN=local`. Regra de versão: usar a versão mais recente cujo `go.mod` declare `go ≤ 1.24`. Nada do Go 1.25: nem `http.CrossOriginProtection`, nem `testing/synctest` estável.

| Uso | Escolha | Por quê |
|---|---|---|
| Roteamento | `net/http.ServeMux` (padrões `"GET /api/v1/x/{id}"`) | Resolve método, parâmetros e precedência. `r.Pattern` serve de rótulo de log e métrica. A tabela única de rotas é nossa, o que dispensa `chi.Walk`. |
| OIDC | `github.com/coreos/go-oidc/v3` ~v3.14 | Faz discovery, JWKS com rotação e verificação de `iss`/`aud`/`exp`. Usamos a allowlist de algoritmos `RS256, PS256, ES256`. |
| OAuth2/PKCE | `golang.org/x/oauth2` ~v0.30 | Usamos `GenerateVerifier`, `S256ChallengeOption`, `VerifierOption` e o `TokenSource` para refresh. |
| Sessão | Código próprio (`internal/auth/session.go`) | A sessão fica no MySQL com o sha256 do token, revogação, idle/absoluto e recheck de papéis. |
| Driver MySQL | `github.com/go-sql-driver/mysql` ~v1.9 | É o padrão. Usamos `RegisterTLSConfig` para a CA do DBaaS e `maxAllowedPacket=0`. |
| Migrations | Runner próprio (~150 linhas) sobre `embed.FS` | Tem `GET_LOCK`, checksum, `dirty` e `force`. DDL no MySQL não é transacional em nenhuma ferramenta. |
| Validação | `httpx.V` próprio: Required, MaxLen, Email, OneOf, HTTPURL, Range, Date | As regras são poucas e explícitas. Invariantes ficam no `model.go`. |
| Logs | `log/slog` em JSON no stdout | Stdlib. |
| Rate limit | `golang.org/x/time/rate` ~v0.12 | Em memória, por unidade. |
| Métricas | `github.com/prometheus/client_golang` ~v1.22 | `/metrics` com bearer token. |
| S3 (opcional) | `github.com/minio/minio-go/v7` | Cliente S3-compatível enxuto. Só é usado com `FILES_BACKEND=s3`. |
| JWT do IdP mock | `github.com/go-jose/go-jose/v4` | Já é dependência transitiva do go-oidc. Só `internal/devidp` importa. |
| Fuso | `time/tzdata` importado em `cmd/server` | Dispensa tzdata na imagem. |
| Testes | `testing` + `httptest`; `github.com/johannesboyne/gofakes3` (só em teste) | Sem testify nem testcontainers, porque o daemon Docker não roda aqui. |
| Ferramentas | `go tool` (diretiva `tool` do Go 1.24): `honnef.co/go/tools/cmd/staticcheck@2025.1.x` e `golang.org/x/vuln/cmd/govulncheck@v1.1.x` | Rodam em `make lint`. |

Recusados: ORM (GORM/ent), sqlc, sqlx/squirrel, validator, chi, scs, Redis, fila, worker separado, outbox, OpenAPI com geração de código, cifra de campo e cadeia de hash na auditoria, robfig/cron.

### 2.2 Frontend (Node 22, pnpm 10.28)

| Uso | Escolha |
|---|---|
| Build | `vite` ~7, `@vitejs/plugin-react` ~5, `typescript` ~5.9 (strict, travado no projeto) |
| React | `react` / `react-dom` ~19.1 |
| UI | `@mantine/core`, `hooks`, `form`, `dates`, `notifications`, `dropzone` ~8; `dayjs` (locale pt-br); `@tabler/icons-react` ~3; `postcss-preset-mantine` + `postcss-simple-vars`. Não usamos `ColorSchemeScript`, porque é script inline e quebraria a CSP. |
| Roteamento | `react-router` ~7 (`createBrowserRouter`, rotas lazy). Os filtros ficam na URL (`useSearchParams`). |
| Dados | `@tanstack/react-query` ~5, com cliente `fetch` próprio. Cache só em memória e `queryClient.clear()` no logout. |
| Formulários | `@mantine/form`. A validação de verdade é a do servidor: o 422 vira `form.setErrors`. |
| Testes unitários e de componente | `vitest` ~3.2, `@testing-library/react` ~16, `@testing-library/user-event` ~14, `@testing-library/jest-dom` ~6, `jsdom` ~26 (setup com polyfills `matchMedia` e `ResizeObserver` para o Mantine) |
| E2E | `@playwright/test` **1.56.1 exato**, para casar com `chromium-1194`, e `@axe-core/playwright` ~4.10 |
| Lint | `eslint` 9 (flat), `typescript-eslint` 8, `eslint-plugin-react-hooks` 5. A regra `no-restricted-syntax` proíbe `dangerouslySetInnerHTML`. |

Recusados no frontend: Redux/Zustand, zod, react-hook-form, MSW (o `fetch` é mockado por helper), biblioteca de gráficos (a matriz é um grid CSS 3×3) e tabelas avançadas.

---

## 3. Modelo de dados

### 3.1 Convenções

- **Banco:** MySQL ≥ 8.0.16. Na subida, o app confere `VERSION()` e recusa versões anteriores, porque o `CHECK` precisa ser aplicado. Engine InnoDB, `utf8mb4` / `utf8mb4_0900_ai_ci`, que dá busca sem acento e sem caixa.
- **Chaves:** PK `id BIGINT UNSIGNED AUTO_INCREMENT`. A FK é sempre uma constraint de tabela nomeada, porque o MySQL ignora `REFERENCES` declarado na própria coluna. O padrão é `RESTRICT`, salvo `CASCADE` indicado.
- **Tempo:** `*_at` é `DATETIME(3)` em UTC; o DSN fixa `time_zone='+00:00'` e `loc=UTC`. `*_on` é `DATE` de negócio, no fuso `APP_TZ` (America/Sao_Paulo).
- **Tipos:**
  - dinheiro em `BIGINT UNSIGNED` centavos (`*_cents`);
  - enums como `VARCHAR + CHECK`, espelhados em constantes Go;
  - `version INT UNSIGNED` para lock otimista.
- **Restrições do MySQL a respeitar:**
  - colunas usadas em `CHECK` não podem ter FK com ação referencial;
  - colunas-base de coluna gerada `STORED` só aceitam FK com `RESTRICT`.
- **Partial unique** ("só um ativo") é feito com uma coluna gerada `IF(cond, chave, NULL)` mais `UNIQUE`.
- **Auditoria (§3.10):** `audit_log` só guarda campos `plain`. PII de candidato, dados restritos e o conteúdo do Nine Box são `redacted`. Por isso a anonimização não precisa reescrever a auditoria, e a tabela é append-only de fato.

### 3.2 Migrations e runner

- **Arquivos:** `migrations/NNNN_nome.sql`, com `version = NNNN`, embutidos via `//go:embed *.sql`. Há um arquivo por marco, com `CREATE TABLE IF NOT EXISTS`, o que torna seguro reexecutar depois de uma correção.
- **Regras de evolução:**
  - qualquer `ALTER`, `DROP` ou migração de dados futura vai sozinha em seu próprio arquivo;
  - um arquivo já mergeado nunca é editado, e o checksum garante isso;
  - toda migration é compatível com a versão anterior do app (expand/contract), para permitir rolling deploy e rollback.
- **`migrate.Up(ctx, dsn)`:**
  1. Abre um `*sql.DB` dedicado com `multiStatements=true` e `MaxOpen=1`, usando `MIGRATION_DATABASE_DSN` ou o DSN da app. O pool da aplicação nunca usa `multiStatements`.
  2. Faz `SELECT GET_LOCK('pp:schema_migrate', 120)` e exige retorno 1.
  3. Cria `schema_migrations`.
  4. Se houver linha com `dirty=1`, para com erro e instrui a rodar `server migrate force N`.
  5. Confere o checksum de cada versão aplicada.
  6. Para cada arquivo pendente, em ordem: `INSERT (version,name,checksum,dirty=1)`, `Exec(conteúdo)`, `UPDATE dirty=0, applied_at`.
  7. `RELEASE_LOCK`.
- **`server migrate force N`:** marca N como aplicada e limpa, com o checksum atual. Serve para depois de uma correção manual.
- **`serve` com `MIGRATE_ON_START=false`:** se o schema estiver atrás do binário ou `dirty`, sai com código 1. Se estiver à frente (rollback do app), registra warning e segue.

```sql
-- criada pelo runner
CREATE TABLE IF NOT EXISTS schema_migrations (
  version    INT UNSIGNED NOT NULL,
  name       VARCHAR(200) NOT NULL,
  checksum   BINARY(32)   NOT NULL,
  dirty      BOOLEAN      NOT NULL DEFAULT 1,
  started_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  applied_at DATETIME(3)  NULL,
  PRIMARY KEY (version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 3.3 `0001_core.sql` (M1)

```sql
SET FOREIGN_KEY_CHECKS = 0;  -- ciclo teams↔people↔users; sessão dedicada da migração

CREATE TABLE IF NOT EXISTS org_units (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  code       VARCHAR(32)  NOT NULL,
  name       VARCHAR(200) NOT NULL,
  active     BOOLEAN      NOT NULL DEFAULT 1,
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_org_units_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS job_titles (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  code       VARCHAR(32)  NOT NULL,
  name       VARCHAR(200) NOT NULL,
  active     BOOLEAN      NOT NULL DEFAULT 1,
  source     VARCHAR(8)   NOT NULL DEFAULT 'erp',
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_job_titles_code (code),
  CONSTRAINT ck_job_titles_source CHECK (source IN ('erp','manual'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS teams (
  id                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  org_unit_id       BIGINT UNSIGNED NOT NULL,
  code              VARCHAR(32)  NOT NULL,
  name              VARCHAR(200) NOT NULL,
  manager_person_id BIGINT UNSIGNED NULL,          -- gestor declarado (CSV ERP ou Admin); compõe ManagedTeams
  active            BOOLEAN      NOT NULL DEFAULT 1,
  source            VARCHAR(8)   NOT NULL DEFAULT 'erp',
  created_at        DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at        DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_teams_code (org_unit_id, code),
  KEY ix_teams_manager (manager_person_id),
  KEY ix_teams_org_active (org_unit_id, active),
  CONSTRAINT ck_teams_source CHECK (source IN ('erp','manual')),
  CONSTRAINT fk_teams_org     FOREIGN KEY (org_unit_id)       REFERENCES org_units(id),
  CONSTRAINT fk_teams_manager FOREIGN KEY (manager_person_id) REFERENCES people(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS people (
  id                          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  org_unit_id                 BIGINT UNSIGNED NOT NULL,
  team_id                     BIGINT UNSIGNED NULL,
  employee_number             VARCHAR(32)  NOT NULL,     -- matrícula ERP
  full_name                   VARCHAR(200) NOT NULL,
  email                       VARCHAR(254) NULL,
  email_norm                  VARCHAR(254) NULL,         -- lower(trim(email))
  job_title_id                BIGINT UNSIGNED NULL,
  erp_manager_person_id       BIGINT UNSIGNED NULL,
  manager_override_person_id  BIGINT UNSIGNED NULL,      -- RF-AUTH-03
  manager_override_reason     VARCHAR(500) NULL,
  manager_override_by         BIGINT UNSIGNED NULL,
  manager_override_at         DATETIME(3) NULL,
  effective_manager_person_id BIGINT UNSIGNED
      AS (COALESCE(manager_override_person_id, erp_manager_person_id)) STORED,
  cost_center                 VARCHAR(32) NULL,
  hired_on                    DATE NULL,
  active                      BOOLEAN NOT NULL DEFAULT 1,
  source                      VARCHAR(8) NOT NULL DEFAULT 'erp',
  synced_at                   DATETIME(3) NULL,
  created_at                  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at                  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_people_employee (employee_number),
  UNIQUE KEY uq_people_email (email_norm),
  KEY ix_people_manager (effective_manager_person_id, active),
  KEY ix_people_team (team_id, active),
  KEY ix_people_org_name (org_unit_id, full_name),
  CONSTRAINT ck_people_source   CHECK (source IN ('erp','manual')),
  CONSTRAINT ck_people_override CHECK (manager_override_person_id IS NULL OR manager_override_reason IS NOT NULL),
  CONSTRAINT fk_people_org      FOREIGN KEY (org_unit_id)                REFERENCES org_units(id),
  CONSTRAINT fk_people_team     FOREIGN KEY (team_id)                    REFERENCES teams(id),
  CONSTRAINT fk_people_title    FOREIGN KEY (job_title_id)               REFERENCES job_titles(id),
  CONSTRAINT fk_people_erp_mgr  FOREIGN KEY (erp_manager_person_id)      REFERENCES people(id),
  CONSTRAINT fk_people_ovr_mgr  FOREIGN KEY (manager_override_person_id) REFERENCES people(id),
  CONSTRAINT fk_people_ovr_by   FOREIGN KEY (manager_override_by)        REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS users (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  oidc_issuer   VARCHAR(255) NOT NULL,
  oidc_subject  VARCHAR(255) NOT NULL,
  email         VARCHAR(254) NOT NULL,
  email_norm    VARCHAR(254) NOT NULL,
  display_name  VARCHAR(200) NOT NULL,
  person_id     BIGINT UNSIGNED NULL,
  person_link   VARCHAR(8) NULL,                          -- como o vínculo foi feito
  last_roles    SET('manager','head','admin') NOT NULL DEFAULT '',
  last_login_at DATETIME(3) NULL,
  disabled_at   DATETIME(3) NULL,                         -- bloqueio pelo Admin
  disabled_by   BIGINT UNSIGNED NULL,
  created_at    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_users_subject (oidc_issuer, oidc_subject),
  UNIQUE KEY uq_users_person (person_id),
  KEY ix_users_email (email_norm),
  CONSTRAINT ck_users_link    CHECK (person_link IS NULL OR person_link IN ('employee','email','manual')),
  CONSTRAINT fk_users_person   FOREIGN KEY (person_id)   REFERENCES people(id),
  CONSTRAINT fk_users_disabler FOREIGN KEY (disabled_by) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS sessions (
  token_hash            BINARY(32) NOT NULL,              -- sha256(token do cookie)
  user_id               BIGINT UNSIGNED NOT NULL,
  roles                 SET('manager','head','admin') NOT NULL,
  refresh_token_enc     BLOB NULL,                        -- seal(AES-GCM), AAD "rt|<hash>"
  id_token_enc          BLOB NULL,                        -- seal, só p/ id_token_hint no logout
  created_at            DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  last_seen_at          DATETIME(3) NOT NULL,
  absolute_expires_at   DATETIME(3) NOT NULL,
  roles_checked_at      DATETIME(3) NOT NULL,
  recheck_failing_since DATETIME(3) NULL,
  ip                    VARCHAR(45)  NULL,
  user_agent            VARCHAR(255) NULL,
  PRIMARY KEY (token_hash),
  KEY ix_sessions_user (user_id),
  KEY ix_sessions_abs (absolute_expires_at),
  KEY ix_sessions_seen (last_seen_at),
  CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS app_settings (               -- configs e feature flags ("feature.*"); defaults em Go
  setting_key VARCHAR(100) NOT NULL,
  value       JSON NOT NULL,
  updated_by  BIGINT UNSIGNED NULL,
  updated_at  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (setting_key),
  CONSTRAINT fk_settings_user FOREIGN KEY (updated_by) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS audit_log (                  -- append-only; a app nunca faz UPDATE/DELETE
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  occurred_at   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  org_unit_id   BIGINT UNSIGNED NULL,
  actor_user_id BIGINT UNSIGNED NULL,                     -- NULL = sistema/job
  action        VARCHAR(64) NOT NULL,                     -- ex.: candidate.transition, quarter.close
  entity_type   VARCHAR(40) NOT NULL,
  entity_id     BIGINT UNSIGNED NULL,
  subject_type  VARCHAR(16) NULL,                         -- titular afetado
  subject_id    BIGINT UNSIGNED NULL,
  before_json   JSON NULL,                                -- só campos plain; redacted = {"changed":true}
  after_json    JSON NULL,
  reason        VARCHAR(1000) NULL,                       -- só ações administrativas (override, anonimização)
  request_id    CHAR(16) NULL,
  ip            VARCHAR(45) NULL,
  PRIMARY KEY (id),
  KEY ix_audit_entity (entity_type, entity_id, occurred_at),
  KEY ix_audit_subject (subject_type, subject_id),
  KEY ix_audit_actor (actor_user_id, occurred_at),
  KEY ix_audit_action (action, occurred_at),
  KEY ix_audit_time (occurred_at),
  CONSTRAINT ck_audit_subject CHECK (subject_type IS NULL OR subject_type IN ('candidate','person')),
  CONSTRAINT fk_audit_actor FOREIGN KEY (actor_user_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS access_log (                 -- RNF-06: 1 linha por titular exposto
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  occurred_at   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  user_id       BIGINT UNSIGNED NOT NULL,
  action        VARCHAR(48) NOT NULL,                     -- ninebox.matrix.view, candidate.restricted.view, attachment.download, candidate.export
  resource_type VARCHAR(40) NOT NULL,
  resource_id   BIGINT UNSIGNED NULL,
  subject_type  VARCHAR(16) NOT NULL,
  subject_id    BIGINT UNSIGNED NOT NULL,
  request_id    CHAR(16) NULL,
  ip            VARCHAR(45) NULL,
  PRIMARY KEY (id),
  KEY ix_access_subject (subject_type, subject_id, occurred_at),
  KEY ix_access_user (user_id, occurred_at),
  KEY ix_access_time (occurred_at),
  CONSTRAINT ck_access_subject CHECK (subject_type IN ('candidate','person')),
  CONSTRAINT fk_access_user FOREIGN KEY (user_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS job_runs (
  id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  job_name     VARCHAR(64) NOT NULL,
  slot_at      DATETIME(3) NOT NULL,                      -- instante agendado (manual = NOW(3))
  origin       VARCHAR(8)  NOT NULL,
  unit         VARCHAR(128) NOT NULL,                     -- hostname da unidade
  status       VARCHAR(8)  NOT NULL,
  started_at   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  finished_at  DATETIME(3) NULL,
  error        TEXT NULL,
  stats        JSON NULL,
  triggered_by BIGINT UNSIGNED NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_job_runs_slot (job_name, slot_at),        -- 1 execução por slot entre N unidades
  KEY ix_job_runs_recent (job_name, started_at),
  CONSTRAINT ck_job_runs_origin CHECK (origin IN ('schedule','manual')),
  CONSTRAINT ck_job_runs_status CHECK (status IN ('running','ok','error')),
  CONSTRAINT fk_job_runs_user FOREIGN KEY (triggered_by) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS notifications (              -- in-app (RF-REC-10, RF-TRN-06)
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id    BIGINT UNSIGNED NOT NULL,
  kind       VARCHAR(48) NOT NULL,
  title      VARCHAR(200) NOT NULL,
  body       VARCHAR(1000) NULL,
  link_path  VARCHAR(255) NULL,                           -- rota da SPA
  ref_type   VARCHAR(24) NULL,                            -- candidate|training_request|ninebox_entry|quarter|lgpd_request
  ref_id     BIGINT UNSIGNED NULL,
  dedupe_key VARCHAR(191) NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  read_at    DATETIME(3) NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_notif_dedupe (dedupe_key),
  KEY ix_notif_user (user_id, read_at, created_at),
  KEY ix_notif_ref (ref_type, ref_id),
  CONSTRAINT fk_notif_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS user_activity_days (         -- métrica "gestores ativos/mês"
  user_id     BIGINT UNSIGNED NOT NULL,
  activity_on DATE NOT NULL,
  PRIMARY KEY (user_id, activity_on),
  CONSTRAINT fk_activity_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

SET FOREIGN_KEY_CHECKS = 1;
```

### 3.4 `0002_org_catalogs.sql` (M3)

```sql
CREATE TABLE IF NOT EXISTS erp_import_batches (         -- RNF-13: prévia → aplicar/descartar/reprocessar
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  org_unit_id BIGINT UNSIGNED NOT NULL,
  status      VARCHAR(10) NOT NULL DEFAULT 'previewed',
  filename    VARCHAR(255) NOT NULL,
  sha256      BINARY(32) NOT NULL,
  content     MEDIUMBLOB NULL,                            -- CSV original; anulado após 30 dias
  diff_hash   BINARY(32) NULL,                            -- hash do diff mostrado na prévia
  summary     JSON NULL,                                  -- {create,update,deactivate,unchanged,relinkedUsers}
  errors      JSON NULL,                                  -- [{line, field, message}]
  created_by  BIGINT UNSIGNED NOT NULL,
  created_at  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  applied_by  BIGINT UNSIGNED NULL,
  applied_at  DATETIME(3) NULL,
  PRIMARY KEY (id),
  KEY ix_erp_batches_org (org_unit_id, created_at),
  CONSTRAINT ck_erp_batches_status CHECK (status IN ('previewed','applied','discarded','failed')),
  CONSTRAINT fk_erp_batches_org     FOREIGN KEY (org_unit_id) REFERENCES org_units(id),
  CONSTRAINT fk_erp_batches_creator FOREIGN KEY (created_by)  REFERENCES users(id),
  CONSTRAINT fk_erp_batches_applier FOREIGN KEY (applied_by)  REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS skills (                     -- catálogo controlado de stacks
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name       VARCHAR(80) NOT NULL,
  name_norm  VARCHAR(80) NOT NULL,
  category   VARCHAR(16) NOT NULL,
  active     BOOLEAN NOT NULL DEFAULT 1,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_skills_name (name_norm),
  CONSTRAINT ck_skills_category CHECK (category IN ('language','framework','cloud','database','messaging','observability','iac','other'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS competencies (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name        VARCHAR(120) NOT NULL,
  description TEXT NULL,
  position    INT NOT NULL DEFAULT 0,
  active      BOOLEAN NOT NULL DEFAULT 1,
  created_at  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_competencies_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 3.5 `0003_recruiting.sql` (M4)

```sql
CREATE TABLE IF NOT EXISTS job_openings (               -- vagas (RF-REC-04 "vagas relacionadas", RF-REC-09)
  id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  org_unit_id     BIGINT UNSIGNED NOT NULL,
  title           VARCHAR(200) NOT NULL,
  team_id         BIGINT UNSIGNED NULL,
  status          VARCHAR(8) NOT NULL DEFAULT 'open',
  external_source VARCHAR(16) NULL,
  external_id     VARCHAR(64) NULL,
  created_by      BIGINT UNSIGNED NULL,                   -- NULL = importado
  created_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_openings_ext (external_source, external_id),
  KEY ix_openings_org (org_unit_id, status),
  CONSTRAINT ck_openings_status CHECK (status IN ('open','closed')),
  CONSTRAINT ck_openings_ext    CHECK (external_source IS NULL OR external_source IN ('gupy')),
  CONSTRAINT fk_openings_org  FOREIGN KEY (org_unit_id) REFERENCES org_units(id),
  CONSTRAINT fk_openings_team FOREIGN KEY (team_id)     REFERENCES teams(id),
  CONSTRAINT fk_openings_user FOREIGN KEY (created_by)  REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS candidates (
  id                   BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  org_unit_id          BIGINT UNSIGNED NOT NULL,
  full_name            VARCHAR(200) NOT NULL,
  email                VARCHAR(254) NULL,
  email_norm           VARCHAR(254) NULL,
  phone                VARCHAR(40)  NULL,
  linkedin_url         VARCHAR(500) NULL,
  linkedin_handle      VARCHAR(200) NULL,                 -- normalizado "in/joao-silva" (dedup)
  github_url           VARCHAR(500) NULL,
  portfolio_url        VARCHAR(500) NULL,
  city                 VARCHAR(120) NULL,
  state                CHAR(2) NULL,
  work_models          SET('remote','hybrid','onsite') NOT NULL DEFAULT '',
  available_from       DATE NULL,
  notice_period_days   SMALLINT UNSIGNED NULL,
  desired_job_title_id BIGINT UNSIGNED NULL,
  seniority            VARCHAR(16) NULL,
  profile              VARCHAR(16) NULL,
  interest_team_id     BIGINT UNSIGNED NULL,
  years_experience     DECIMAL(4,1) NULL,
  languages            JSON NULL,                         -- [{"language":"en","level":"advanced"}]
  is_former_employee   BOOLEAN NOT NULL DEFAULT 0,
  current_company      VARCHAR(200) NULL,
  current_title        VARCHAR(200) NULL,
  source               VARCHAR(16) NOT NULL,
  referrer_person_id   BIGINT UNSIGNED NULL,              -- quem indicou (colaborador ou gestor)
  referrer_name        VARCHAR(200) NULL,                 -- indicação de fora do ERP
  source_detail        VARCHAR(255) NULL,
  status               VARCHAR(16) NOT NULL DEFAULT 'talent_pool',
  status_changed_at    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  owner_user_id        BIGINT UNSIGNED NOT NULL,          -- responsável (avisos LGPD); = created_by na criação
  legal_basis          VARCHAR(24) NOT NULL,
  consent_obtained_on  DATE NULL,
  data_collected_on    DATE NOT NULL,
  retention_expires_on DATE NOT NULL,
  retention_warned_at  DATETIME(3) NULL,
  archived_at          DATETIME(3) NULL,
  archived_by          BIGINT UNSIGNED NULL,
  anonymized_at        DATETIME(3) NULL,
  external_source      VARCHAR(16) NULL,
  external_id          VARCHAR(64) NULL,
  search_text          VARCHAR(2000) NOT NULL DEFAULT '',  -- mantido pela app (nome, e-mail, empresa, cargo, cidade, tags, skills, vagas)
  version              INT UNSIGNED NOT NULL DEFAULT 1,
  created_by           BIGINT UNSIGNED NOT NULL,
  updated_by           BIGINT UNSIGNED NOT NULL,
  created_at           DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at           DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_cand_ext (external_source, external_id),
  KEY ix_cand_list (org_unit_id, anonymized_at, status, updated_at),
  KEY ix_cand_email (email_norm),
  KEY ix_cand_linkedin (linkedin_handle),
  KEY ix_cand_retention (retention_expires_on, anonymized_at),
  KEY ix_cand_owner (owner_user_id),
  KEY ix_cand_created (org_unit_id, created_at),
  CONSTRAINT ck_cand_seniority CHECK (seniority IS NULL OR seniority IN ('jr','pl','sr','specialist','staff')),
  CONSTRAINT ck_cand_profile   CHECK (profile IS NULL OR profile IN ('backend','frontend','fullstack','mobile','data','sre_platform','qa','security','management','other')),
  CONSTRAINT ck_cand_source    CHECK (source IN ('referral','direct','ats','event','hunting')),
  CONSTRAINT ck_cand_status    CHECK (status IN ('talent_pool','in_process','approved','hired','rejected','withdrawn')),
  CONSTRAINT ck_cand_basis     CHECK (legal_basis IN ('consent','legitimate_interest','pre_contract')),
  CONSTRAINT ck_cand_consent   CHECK (legal_basis <> 'consent' OR consent_obtained_on IS NOT NULL OR anonymized_at IS NOT NULL),
  CONSTRAINT ck_cand_ext       CHECK (external_source IS NULL OR external_source IN ('gupy')),
  CONSTRAINT ck_cand_langs     CHECK (languages IS NULL OR JSON_TYPE(languages) = 'ARRAY'),
  CONSTRAINT fk_cand_org      FOREIGN KEY (org_unit_id)          REFERENCES org_units(id),
  CONSTRAINT fk_cand_title    FOREIGN KEY (desired_job_title_id) REFERENCES job_titles(id),
  CONSTRAINT fk_cand_team     FOREIGN KEY (interest_team_id)     REFERENCES teams(id),
  CONSTRAINT fk_cand_referrer FOREIGN KEY (referrer_person_id)   REFERENCES people(id),
  CONSTRAINT fk_cand_owner    FOREIGN KEY (owner_user_id)        REFERENCES users(id),
  CONSTRAINT fk_cand_archiver FOREIGN KEY (archived_by)          REFERENCES users(id),
  CONSTRAINT fk_cand_creator  FOREIGN KEY (created_by)           REFERENCES users(id),
  CONSTRAINT fk_cand_updater  FOREIGN KEY (updated_by)           REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS candidate_skills (
  candidate_id BIGINT UNSIGNED NOT NULL,
  skill_id     BIGINT UNSIGNED NOT NULL,
  level        TINYINT UNSIGNED NOT NULL,
  years        DECIMAL(4,1) NULL,
  PRIMARY KEY (candidate_id, skill_id),
  KEY ix_cskills_filter (skill_id, level, candidate_id),  -- "Go ≥ 4"
  CONSTRAINT ck_cskills_level CHECK (level BETWEEN 1 AND 5),
  CONSTRAINT fk_cskills_cand  FOREIGN KEY (candidate_id) REFERENCES candidates(id) ON DELETE CASCADE,
  CONSTRAINT fk_cskills_skill FOREIGN KEY (skill_id)     REFERENCES skills(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS candidate_tags (             -- tags livres (lower, trim, ≤ 50)
  candidate_id BIGINT UNSIGNED NOT NULL,
  tag          VARCHAR(50) NOT NULL,
  PRIMARY KEY (candidate_id, tag),
  KEY ix_ctags_tag (tag),
  CONSTRAINT fk_ctags_cand FOREIGN KEY (candidate_id) REFERENCES candidates(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS candidate_openings (
  candidate_id   BIGINT UNSIGNED NOT NULL,
  job_opening_id BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (candidate_id, job_opening_id),
  KEY ix_copen_opening (job_opening_id),
  CONSTRAINT fk_copen_cand    FOREIGN KEY (candidate_id)   REFERENCES candidates(id) ON DELETE CASCADE,
  CONSTRAINT fk_copen_opening FOREIGN KEY (job_opening_id) REFERENCES job_openings(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS candidate_events (           -- timeline RF-REC-06; transições RF-REC-04
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  candidate_id  BIGINT UNSIGNED NOT NULL,
  kind          VARCHAR(24) NOT NULL,
  from_status   VARCHAR(16) NULL,
  to_status     VARCHAR(16) NULL,
  body          TEXT NULL,                                -- nota/comentário público da transição
  ref_id        BIGINT UNSIGNED NULL,                     -- entrevista/anexo/vaga/pedido LGPD
  actor_user_id BIGINT UNSIGNED NULL,
  created_at    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY ix_cevents_cand (candidate_id, created_at),
  CONSTRAINT ck_cevents_kind CHECK (kind IN ('created','updated','status_changed','note','interview_added','interview_updated',
     'interview_removed','file_added','file_removed','consent_renewed','owner_changed','archived','unarchived',
     'opening_linked','lgpd_request','anonymized','imported')),
  CONSTRAINT fk_cevents_cand  FOREIGN KEY (candidate_id)  REFERENCES candidates(id),
  CONSTRAINT fk_cevents_actor FOREIGN KEY (actor_user_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS candidate_views (            -- métrica "consultado por gestor ≠ cadastrante"
  candidate_id    BIGINT UNSIGNED NOT NULL,
  user_id         BIGINT UNSIGNED NOT NULL,
  first_viewed_at DATETIME(3) NOT NULL,
  last_viewed_at  DATETIME(3) NOT NULL,
  view_count      INT UNSIGNED NOT NULL DEFAULT 1,
  PRIMARY KEY (candidate_id, user_id),
  CONSTRAINT fk_cviews_cand FOREIGN KEY (candidate_id) REFERENCES candidates(id) ON DELETE CASCADE,
  CONSTRAINT fk_cviews_user FOREIGN KEY (user_id)      REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 3.6 `0004_recruiting_files_lgpd.sql` (M5)

```sql
CREATE TABLE IF NOT EXISTS attachments (                -- metadados; o dono vem das tabelas de junção
  id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  org_unit_id  BIGINT UNSIGNED NOT NULL,
  kind         VARCHAR(16) NOT NULL,
  filename     VARCHAR(255) NOT NULL,                     -- só metadado (basename saneado)
  content_type VARCHAR(100) NOT NULL,                     -- tipo detectado (sniffing), não o declarado
  size_bytes   INT UNSIGNED NOT NULL,
  sha256       BINARY(32) NOT NULL,
  storage      VARCHAR(8) NOT NULL,
  storage_key  VARCHAR(255) NULL,                         -- chave S3 (aleatória)
  scan_status  VARCHAR(12) NOT NULL DEFAULT 'not_scanned',
  uploaded_by  BIGINT UNSIGNED NOT NULL,
  uploaded_at  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  CONSTRAINT ck_att_kind    CHECK (kind IN ('cv','portfolio','technical_test','proposal','quote','invoice','payment_proof','certificate','other')),
  CONSTRAINT ck_att_storage CHECK (storage IN ('mysql','s3')),
  CONSTRAINT ck_att_key     CHECK (storage = 'mysql' OR storage_key IS NOT NULL),
  CONSTRAINT ck_att_scan    CHECK (scan_status IN ('not_scanned','clean','error')),
  CONSTRAINT fk_att_org  FOREIGN KEY (org_unit_id) REFERENCES org_units(id),
  CONSTRAINT fk_att_user FOREIGN KEY (uploaded_by) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS attachment_blobs (           -- separada: listagens nunca tocam o blob
  attachment_id BIGINT UNSIGNED NOT NULL,
  data          MEDIUMBLOB NOT NULL,
  PRIMARY KEY (attachment_id),
  CONSTRAINT fk_blobs_att FOREIGN KEY (attachment_id) REFERENCES attachments(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS candidate_attachments (
  attachment_id BIGINT UNSIGNED NOT NULL,
  candidate_id  BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (attachment_id),
  KEY ix_catt_cand (candidate_id),
  CONSTRAINT fk_catt_att  FOREIGN KEY (attachment_id) REFERENCES attachments(id) ON DELETE CASCADE,
  CONSTRAINT fk_catt_cand FOREIGN KEY (candidate_id)  REFERENCES candidates(id)   -- RESTRICT: código apaga anexos antes
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS candidate_restricted_entries ( -- §4.1: visível ao AUTOR da entrada e ao Head
  id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  candidate_id   BIGINT UNSIGNED NOT NULL,
  kind           VARCHAR(20) NOT NULL,
  salary_cents   BIGINT UNSIGNED NULL,
  body           TEXT NULL,
  author_user_id BIGINT UNSIGNED NOT NULL,
  created_at     DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at     DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY ix_cre_cand (candidate_id, author_user_id),
  CONSTRAINT ck_cre_kind  CHECK (kind IN ('salary_expectation','rejection_reason','sensitive_note')),
  CONSTRAINT ck_cre_value CHECK ((kind = 'salary_expectation' AND salary_cents IS NOT NULL)
                              OR (kind <> 'salary_expectation' AND body IS NOT NULL)),
  CONSTRAINT fk_cre_cand   FOREIGN KEY (candidate_id)   REFERENCES candidates(id),
  CONSTRAINT fk_cre_author FOREIGN KEY (author_user_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS candidate_interviews (       -- RF-REC-03 (CRUD)
  id                    BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  candidate_id          BIGINT UNSIGNED NOT NULL,
  interviewer_person_id BIGINT UNSIGNED NULL,             -- entrevistador do ERP (dev, gestor…)
  interviewer_name      VARCHAR(200) NOT NULL,            -- sempre preenchido (cópia ou externo)
  interviewed_on        DATE NOT NULL,
  kind                  VARCHAR(16) NOT NULL,
  recommendation        VARCHAR(16) NOT NULL,
  opinion               TEXT NULL,                        -- obrigatório na API; NULL só após anonimização
  created_by            BIGINT UNSIGNED NOT NULL,
  created_at            DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at            DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY ix_interviews_cand (candidate_id, interviewed_on),
  CONSTRAINT ck_interviews_kind CHECK (kind IN ('technical','behavioral','culture_fit','case')),
  CONSTRAINT ck_interviews_rec  CHECK (recommendation IN ('strong_yes','yes','no','strong_no')),
  CONSTRAINT fk_interviews_cand    FOREIGN KEY (candidate_id)          REFERENCES candidates(id),
  CONSTRAINT fk_interviews_person  FOREIGN KEY (interviewer_person_id) REFERENCES people(id),
  CONSTRAINT fk_interviews_creator FOREIGN KEY (created_by)            REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS interview_scores (
  interview_id  BIGINT UNSIGNED NOT NULL,
  competency_id BIGINT UNSIGNED NOT NULL,
  score         TINYINT UNSIGNED NOT NULL,
  PRIMARY KEY (interview_id, competency_id),
  CONSTRAINT ck_iscores_score CHECK (score BETWEEN 1 AND 5),
  CONSTRAINT fk_iscores_interview  FOREIGN KEY (interview_id)  REFERENCES candidate_interviews(id) ON DELETE CASCADE,
  CONSTRAINT fk_iscores_competency FOREIGN KEY (competency_id) REFERENCES competencies(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS lgpd_requests (              -- pedidos do titular (RNF-06)
  id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  org_unit_id     BIGINT UNSIGNED NOT NULL,
  candidate_id    BIGINT UNSIGNED NOT NULL,
  kind            VARCHAR(12) NOT NULL,
  channel         VARCHAR(64) NOT NULL,                   -- e-mail, LinkedIn, Gupy…
  received_on     DATE NOT NULL,
  due_on          DATE NOT NULL,                          -- received_on + lgpd.request_sla_days
  details         VARCHAR(1000) NULL,                     -- anulado na anonimização
  status          VARCHAR(10) NOT NULL DEFAULT 'open',
  registered_by   BIGINT UNSIGNED NOT NULL,
  resolved_by     BIGINT UNSIGNED NULL,
  resolved_at     DATETIME(3) NULL,
  resolution_note VARCHAR(1000) NULL,
  created_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY ix_lgpd_status (org_unit_id, status, due_on),
  KEY ix_lgpd_cand (candidate_id),
  CONSTRAINT ck_lgpd_kind   CHECK (kind IN ('erasure','access','correction')),
  CONSTRAINT ck_lgpd_status CHECK (status IN ('open','done','rejected')),
  CONSTRAINT fk_lgpd_org      FOREIGN KEY (org_unit_id)   REFERENCES org_units(id),
  CONSTRAINT fk_lgpd_cand     FOREIGN KEY (candidate_id)  REFERENCES candidates(id),
  CONSTRAINT fk_lgpd_reg      FOREIGN KEY (registered_by) REFERENCES users(id),
  CONSTRAINT fk_lgpd_resolver FOREIGN KEY (resolved_by)   REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 3.7 `0005_training.sql` (M6): quarters, orçamento e pleitos

```sql
CREATE TABLE IF NOT EXISTS quarters (                   -- também carrega o ciclo do Nine Box
  id                            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  org_unit_id                   BIGINT UNSIGNED NOT NULL,
  code                          CHAR(7) NOT NULL,         -- '2026-Q4' (validado em Go)
  starts_on                     DATE NOT NULL,
  ends_on                       DATE NOT NULL,
  training_deadline_on          DATE NULL,                -- janela de submissão (RF-TRN-06)
  training_deadline_notified_at DATETIME(3) NULL,
  ninebox_status                VARCHAR(12) NOT NULL DEFAULT 'planned',
  ninebox_opens_on              DATE NULL,                -- datas-alvo (RF-9BOX-01)
  ninebox_calibration_on        DATE NULL,
  ninebox_closes_on             DATE NULL,
  ninebox_matrix                JSON NULL,                -- snapshot de ninebox.matrix ao ABRIR
  opened_at                     DATETIME(3) NULL,
  opened_by                     BIGINT UNSIGNED NULL,
  calibration_started_at        DATETIME(3) NULL,
  calibration_started_by        BIGINT UNSIGNED NULL,
  closed_at                     DATETIME(3) NULL,
  closed_by                     BIGINT UNSIGNED NULL,
  close_entries_submitted       INT UNSIGNED NULL,        -- cobertura no fechamento (métrica §9)
  close_people_total            INT UNSIGNED NULL,
  live_org_unit_id              BIGINT UNSIGNED
      AS (IF(ninebox_status IN ('open','calibration'), org_unit_id, NULL)) STORED,
  created_by                    BIGINT UNSIGNED NULL,     -- NULL = criado pelo sistema (quarter_ensure)
  created_at                    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at                    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_quarters_code (org_unit_id, code),
  UNIQUE KEY uq_quarters_live (live_org_unit_id),         -- no máx. 1 ciclo vivo por gerência
  KEY ix_quarters_dates (org_unit_id, starts_on),
  CONSTRAINT ck_quarters_dates  CHECK (ends_on > starts_on),
  CONSTRAINT ck_quarters_status CHECK (ninebox_status IN ('planned','open','calibration','closed')),
  CONSTRAINT ck_quarters_nb1    CHECK (ninebox_opens_on IS NULL OR ninebox_calibration_on IS NULL OR ninebox_opens_on <= ninebox_calibration_on),
  CONSTRAINT ck_quarters_nb2    CHECK (ninebox_calibration_on IS NULL OR ninebox_closes_on IS NULL OR ninebox_calibration_on <= ninebox_closes_on),
  CONSTRAINT fk_quarters_org     FOREIGN KEY (org_unit_id)            REFERENCES org_units(id),
  CONSTRAINT fk_quarters_opener  FOREIGN KEY (opened_by)              REFERENCES users(id),
  CONSTRAINT fk_quarters_calib   FOREIGN KEY (calibration_started_by) REFERENCES users(id),
  CONSTRAINT fk_quarters_closer  FOREIGN KEY (closed_by)              REFERENCES users(id),
  CONSTRAINT fk_quarters_creator FOREIGN KEY (created_by)             REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS training_budgets (           -- por gerência (team_id NULL) ou por time (setting)
  id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  quarter_id   BIGINT UNSIGNED NOT NULL,
  team_id      BIGINT UNSIGNED NULL,
  team_key     BIGINT UNSIGNED AS (IFNULL(team_id, 0)) STORED,
  amount_cents BIGINT UNSIGNED NOT NULL,
  updated_by   BIGINT UNSIGNED NOT NULL,
  created_at   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_budgets (quarter_id, team_key),
  CONSTRAINT fk_budgets_quarter FOREIGN KEY (quarter_id) REFERENCES quarters(id),
  CONSTRAINT fk_budgets_team    FOREIGN KEY (team_id)    REFERENCES teams(id),
  CONSTRAINT fk_budgets_user    FOREIGN KEY (updated_by) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS training_requests (
  id                      BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  org_unit_id             BIGINT UNSIGNED NOT NULL,
  quarter_id              BIGINT UNSIGNED NOT NULL,
  team_id                 BIGINT UNSIGNED NOT NULL,
  requester_user_id       BIGINT UNSIGNED NOT NULL,
  title                   VARCHAR(200) NOT NULL,
  provider                VARCHAR(200) NULL,            -- obrigatórios na submissão (validação em Go)
  modality                VARCHAR(16) NULL,
  url                     VARCHAR(1000) NULL,
  content                 TEXT NULL,
  workload_hours          DECIMAL(6,1) NULL,
  starts_on               DATE NULL,
  ends_on                 DATE NULL,
  cost_registration_cents BIGINT UNSIGNED NOT NULL DEFAULT 0,
  cost_travel_cents       BIGINT UNSIGNED NOT NULL DEFAULT 0,
  cost_lodging_cents      BIGINT UNSIGNED NOT NULL DEFAULT 0,
  cost_other_cents        BIGINT UNSIGNED NOT NULL DEFAULT 0,
  cost_total_cents        BIGINT UNSIGNED AS (cost_registration_cents + cost_travel_cents + cost_lodging_cents + cost_other_cents) STORED,
  justification           TEXT NULL,
  expected_gain           TEXT NULL,
  priority                VARCHAR(8) NOT NULL DEFAULT 'medium',
  status                  VARCHAR(12) NOT NULL DEFAULT 'draft',
  submitted_at            DATETIME(3) NULL,
  decided_at              DATETIME(3) NULL,
  decided_by              BIGINT UNSIGNED NULL,
  decision_comment        TEXT NULL,
  cancel_reason           TEXT NULL,
  version                 INT UNSIGNED NOT NULL DEFAULT 1,
  created_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY ix_trq_quarter (org_unit_id, quarter_id, status),
  KEY ix_trq_team (team_id, quarter_id),
  KEY ix_trq_requester (requester_user_id),
  CONSTRAINT ck_trq_modality CHECK (modality IS NULL OR modality IN ('online','in_person','conference','certification')),
  CONSTRAINT ck_trq_priority CHECK (priority IN ('high','medium','low')),
  CONSTRAINT ck_trq_status   CHECK (status IN ('draft','submitted','approved','rejected','paid','in_progress','completed','cancelled')),
  CONSTRAINT ck_trq_dates    CHECK (starts_on IS NULL OR ends_on IS NULL OR ends_on >= starts_on),
  CONSTRAINT fk_trq_org       FOREIGN KEY (org_unit_id)       REFERENCES org_units(id),
  CONSTRAINT fk_trq_quarter   FOREIGN KEY (quarter_id)        REFERENCES quarters(id),
  CONSTRAINT fk_trq_team      FOREIGN KEY (team_id)           REFERENCES teams(id),
  CONSTRAINT fk_trq_requester FOREIGN KEY (requester_user_id) REFERENCES users(id),
  CONSTRAINT fk_trq_decider   FOREIGN KEY (decided_by)        REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS training_request_beneficiaries (
  request_id BIGINT UNSIGNED NOT NULL,
  person_id  BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (request_id, person_id),
  KEY ix_trb_person (person_id),
  CONSTRAINT fk_trb_request FOREIGN KEY (request_id) REFERENCES training_requests(id) ON DELETE CASCADE,
  CONSTRAINT fk_trb_person  FOREIGN KEY (person_id)  REFERENCES people(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS training_transitions (       -- check-in/check-out (RF-TRN-04)
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  request_id    BIGINT UNSIGNED NOT NULL,
  from_status   VARCHAR(12) NULL,                         -- NULL = criação
  to_status     VARCHAR(12) NOT NULL,
  actor_user_id BIGINT UNSIGNED NOT NULL,
  comment       TEXT NULL,
  evidence_ref  VARCHAR(200) NULL,                        -- nº NF, protocolo, link do certificado
  batch_id      BINARY(16) NULL,                          -- decisões em lote
  created_at    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY ix_trt_request (request_id, created_at),
  KEY ix_trt_batch (batch_id),
  CONSTRAINT ck_trt_to   CHECK (to_status IN ('draft','submitted','approved','rejected','paid','in_progress','completed','cancelled')),
  CONSTRAINT ck_trt_from CHECK (from_status IS NULL OR from_status IN ('draft','submitted','approved','rejected','paid','in_progress','completed','cancelled')),
  CONSTRAINT fk_trt_request FOREIGN KEY (request_id)    REFERENCES training_requests(id) ON DELETE CASCADE,
  CONSTRAINT fk_trt_actor   FOREIGN KEY (actor_user_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS training_attachments (
  attachment_id       BIGINT UNSIGNED NOT NULL,
  training_request_id BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (attachment_id),
  KEY ix_tatt_request (training_request_id),
  CONSTRAINT fk_tatt_att     FOREIGN KEY (attachment_id)       REFERENCES attachments(id) ON DELETE CASCADE,
  CONSTRAINT fk_tatt_request FOREIGN KEY (training_request_id) REFERENCES training_requests(id) -- RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS training_transition_evidence (
  transition_id BIGINT UNSIGNED NOT NULL,
  attachment_id BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (transition_id, attachment_id),
  KEY ix_tte_att (attachment_id),
  CONSTRAINT fk_tte_transition FOREIGN KEY (transition_id) REFERENCES training_transitions(id) ON DELETE CASCADE,
  CONSTRAINT fk_tte_att        FOREIGN KEY (attachment_id) REFERENCES attachments(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

Para excluir um rascunho de pleito, tudo numa tx:
1. `DELETE FROM attachments WHERE id IN (SELECT attachment_id FROM training_attachments WHERE training_request_id=?)`. Isso cascateia para blobs, junção e evidências.
2. `DELETE FROM training_requests WHERE id=? AND status='draft'`. Isso cascateia para beneficiários e transições.
3. Depois do commit, apagar os objetos S3.

### 3.8 `0006_ninebox.sql` (M7)

```sql
CREATE TABLE IF NOT EXISTS ninebox_participants (       -- "pares" da calibração (configurável)
  quarter_id BIGINT UNSIGNED NOT NULL,
  person_id  BIGINT UNSIGNED NOT NULL,                    -- por pessoa: inclui gestores que nunca logaram
  added_by   BIGINT UNSIGNED NULL,                        -- NULL = snapshot automático na abertura
  added_at   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (quarter_id, person_id),
  KEY ix_nbp_person (person_id),
  CONSTRAINT fk_nbp_quarter FOREIGN KEY (quarter_id) REFERENCES quarters(id),
  CONSTRAINT fk_nbp_person  FOREIGN KEY (person_id)  REFERENCES people(id),
  CONSTRAINT fk_nbp_user    FOREIGN KEY (added_by)   REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS ninebox_entries (
  id                 BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  quarter_id         BIGINT UNSIGNED NOT NULL,
  person_id          BIGINT UNSIGNED NOT NULL,            -- avaliado
  team_id            BIGINT UNSIGNED NULL,                -- snapshot do time (filtro da matriz)
  manager_person_id  BIGINT UNSIGNED NOT NULL,            -- snapshot do gestor direto na última escrita
  author_user_id     BIGINT UNSIGNED NOT NULL,            -- último editor
  x_level            TINYINT UNSIGNED NULL,               -- eixo X (default Performance) 1..3
  y_level            TINYINT UNSIGNED NULL,               -- eixo Y (default Comportamento) 1..3
  quadrant           TINYINT UNSIGNED AS ((y_level - 1) * 3 + x_level) STORED,  -- 1..9; NULL se incompleto
  justification      TEXT NULL,
  strengths          TEXT NULL,
  development_points TEXT NULL,
  agreed_actions     TEXT NULL,
  status             VARCHAR(10) NOT NULL DEFAULT 'draft',
  submitted_at       DATETIME(3) NULL,
  last_rectified_at  DATETIME(3) NULL,
  version            INT UNSIGNED NOT NULL DEFAULT 1,
  created_at         DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at         DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_nbe_quarter_person (quarter_id, person_id),
  KEY ix_nbe_matrix (quarter_id, status, quadrant),
  KEY ix_nbe_person (person_id, quarter_id),
  KEY ix_nbe_author (author_user_id),
  KEY ix_nbe_manager (manager_person_id, quarter_id),
  CONSTRAINT ck_nbe_x      CHECK (x_level IS NULL OR x_level BETWEEN 1 AND 3),
  CONSTRAINT ck_nbe_y      CHECK (y_level IS NULL OR y_level BETWEEN 1 AND 3),
  CONSTRAINT ck_nbe_status CHECK (status IN ('draft','submitted')),
  CONSTRAINT ck_nbe_submit CHECK (status = 'draft' OR (x_level IS NOT NULL AND y_level IS NOT NULL AND justification IS NOT NULL)),
  CONSTRAINT fk_nbe_quarter FOREIGN KEY (quarter_id)        REFERENCES quarters(id),
  CONSTRAINT fk_nbe_person  FOREIGN KEY (person_id)         REFERENCES people(id),
  CONSTRAINT fk_nbe_team    FOREIGN KEY (team_id)           REFERENCES teams(id),
  CONSTRAINT fk_nbe_manager FOREIGN KEY (manager_person_id) REFERENCES people(id),
  CONSTRAINT fk_nbe_author  FOREIGN KEY (author_user_id)    REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS ninebox_entry_events (       -- antes → depois, autor, motivo (RF-9BOX-03/04)
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  entry_id      BIGINT UNSIGNED NOT NULL,
  kind          VARCHAR(12) NOT NULL,
  actor_user_id BIGINT UNSIGNED NOT NULL,
  reason        VARCHAR(1000) NULL,                       -- obrigatório em adjusted/calibrated/rectified
  before_json   JSON NULL,                                -- {xLevel,yLevel,quadrant,justification,strengths,developmentPoints,agreedActions}
  after_json    JSON NULL,
  created_at    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY ix_nbev_entry (entry_id, created_at),
  CONSTRAINT ck_nbev_kind CHECK (kind IN ('submitted','adjusted','calibrated','rectified')),
  CONSTRAINT fk_nbev_entry FOREIGN KEY (entry_id)      REFERENCES ninebox_entries(id),
  CONSTRAINT fk_nbev_actor FOREIGN KEY (actor_user_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS ninebox_comments (           -- imutáveis
  id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  entry_id       BIGINT UNSIGNED NOT NULL,
  author_user_id BIGINT UNSIGNED NOT NULL,
  body           TEXT NOT NULL,
  created_at     DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY ix_nbc_entry (entry_id, created_at),
  CONSTRAINT fk_nbc_entry  FOREIGN KEY (entry_id)       REFERENCES ninebox_entries(id),
  CONSTRAINT fk_nbc_author FOREIGN KEY (author_user_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 3.9 `0007_onboarding.sql` (M8)

```sql
CREATE TABLE IF NOT EXISTS onboarding_modules (
  id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  org_unit_id     BIGINT UNSIGNED NOT NULL,
  scope           VARCHAR(8) NOT NULL,
  team_id         BIGINT UNSIGNED NULL,
  title           VARCHAR(200) NOT NULL,
  description     TEXT NULL,
  position        INT NOT NULL DEFAULT 0,
  current_version INT UNSIGNED NOT NULL DEFAULT 1,        -- ++ a cada mutação do módulo/itens
  archived_at     DATETIME(3) NULL,
  created_by      BIGINT UNSIGNED NOT NULL,
  updated_by      BIGINT UNSIGNED NOT NULL,
  created_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY ix_onbm_list (org_unit_id, scope, team_id, position),
  CONSTRAINT ck_onbm_scope CHECK ((scope = 'common' AND team_id IS NULL) OR (scope = 'team' AND team_id IS NOT NULL)),
  CONSTRAINT fk_onbm_org     FOREIGN KEY (org_unit_id) REFERENCES org_units(id),
  CONSTRAINT fk_onbm_team    FOREIGN KEY (team_id)     REFERENCES teams(id),
  CONSTRAINT fk_onbm_creator FOREIGN KEY (created_by)  REFERENCES users(id),
  CONSTRAINT fk_onbm_updater FOREIGN KEY (updated_by)  REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS onboarding_module_items (
  id                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  module_id         BIGINT UNSIGNED NOT NULL,
  title             VARCHAR(200) NOT NULL,
  description       TEXT NULL,
  kind              VARCHAR(16) NOT NULL,
  url               VARCHAR(1000) NULL,                   -- só http/https
  responsible_label VARCHAR(120) NULL,                    -- "Buddy", "Gestor", "SRE"…
  due_offset_days   SMALLINT UNSIGNED NOT NULL,           -- D+1, D+7, D+30, D+90
  required          BOOLEAN NOT NULL DEFAULT 1,
  position          INT NOT NULL DEFAULT 0,
  archived_at       DATETIME(3) NULL,
  created_at        DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at        DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY ix_onbi_module (module_id, position),
  CONSTRAINT ck_onbi_kind   CHECK (kind IN ('reading','video','meeting','practice','access_request')),
  CONSTRAINT ck_onbi_offset CHECK (due_offset_days <= 365),
  CONSTRAINT fk_onbi_module FOREIGN KEY (module_id) REFERENCES onboarding_modules(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS onboarding_module_revisions ( -- versionamento real (RF-ONB-01)
  module_id      BIGINT UNSIGNED NOT NULL,
  version        INT UNSIGNED NOT NULL,
  snapshot       JSON NOT NULL,                           -- módulo + itens ativos naquela versão
  change_summary VARCHAR(255) NOT NULL,                   -- "item adicionado: …", "reordenado"…
  created_by     BIGINT UNSIGNED NOT NULL,
  created_at     DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (module_id, version),
  CONSTRAINT fk_onbr_module FOREIGN KEY (module_id)  REFERENCES onboarding_modules(id),
  CONSTRAINT fk_onbr_user   FOREIGN KEY (created_by) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS onboarding_plans (
  id                  BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  org_unit_id         BIGINT UNSIGNED NOT NULL,
  status              VARCHAR(10) NOT NULL DEFAULT 'draft',
  onboardee_name      VARCHAR(200) NOT NULL,
  onboardee_email     VARCHAR(254) NULL,
  person_id           BIGINT UNSIGNED NULL,               -- vinculado quando chegar do ERP
  candidate_id        BIGINT UNSIGNED NULL,               -- RF-REC-08/ONB-07
  job_title_id        BIGINT UNSIGNED NULL,
  team_id             BIGINT UNSIGNED NOT NULL,
  manager_user_id     BIGINT UNSIGNED NOT NULL,           -- responsável pelo plano
  buddy_person_id     BIGINT UNSIGNED NULL,
  start_date          DATE NULL,
  notes               TEXT NULL,                          -- interno; nunca vai para link/export
  composer_started_at DATETIME(3) NULL,                   -- métrica "tempo para montar plano"
  activated_at        DATETIME(3) NULL,
  completed_at        DATETIME(3) NULL,
  cancelled_at        DATETIME(3) NULL,
  cancel_reason       VARCHAR(500) NULL,
  version             INT UNSIGNED NOT NULL DEFAULT 1,
  created_by          BIGINT UNSIGNED NOT NULL,
  created_at          DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at          DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_onbp_candidate (candidate_id),
  KEY ix_onbp_manager (manager_user_id, status),
  KEY ix_onbp_status (org_unit_id, status, start_date),
  CONSTRAINT ck_onbp_status CHECK (status IN ('draft','active','completed','cancelled')),
  CONSTRAINT ck_onbp_start  CHECK (status IN ('draft','cancelled') OR start_date IS NOT NULL),
  CONSTRAINT fk_onbp_org       FOREIGN KEY (org_unit_id)     REFERENCES org_units(id),
  CONSTRAINT fk_onbp_person    FOREIGN KEY (person_id)       REFERENCES people(id),
  CONSTRAINT fk_onbp_candidate FOREIGN KEY (candidate_id)    REFERENCES candidates(id),
  CONSTRAINT fk_onbp_title     FOREIGN KEY (job_title_id)    REFERENCES job_titles(id),
  CONSTRAINT fk_onbp_team      FOREIGN KEY (team_id)         REFERENCES teams(id),
  CONSTRAINT fk_onbp_manager   FOREIGN KEY (manager_user_id) REFERENCES users(id),
  CONSTRAINT fk_onbp_buddy     FOREIGN KEY (buddy_person_id) REFERENCES people(id),
  CONSTRAINT fk_onbp_creator   FOREIGN KEY (created_by)      REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS onboarding_plan_items (
  id                    BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  plan_id               BIGINT UNSIGNED NOT NULL,
  section_title         VARCHAR(200) NOT NULL,            -- título do módulo de origem ou "Itens avulsos"
  source_module_id      BIGINT UNSIGNED NULL,
  source_module_version INT UNSIGNED NULL,
  source_item_id        BIGINT UNSIGNED NULL,
  title                 VARCHAR(200) NOT NULL,
  description           TEXT NULL,
  kind                  VARCHAR(16) NOT NULL,
  url                   VARCHAR(1000) NULL,
  responsible_label     VARCHAR(120) NULL,
  responsible_person_id BIGINT UNSIGNED NULL,
  due_offset_days       SMALLINT UNSIGNED NULL,
  due_date              DATE NOT NULL,
  due_date_overridden   BOOLEAN NOT NULL DEFAULT 0,
  required              BOOLEAN NOT NULL DEFAULT 1,
  position              INT NOT NULL DEFAULT 0,
  status                VARCHAR(16) NOT NULL DEFAULT 'pending',
  status_changed_at     DATETIME(3) NULL,
  status_changed_by     BIGINT UNSIGNED NULL,
  created_at            DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at            DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY ix_onbpi_plan (plan_id, position),
  KEY ix_onbpi_due (status, due_date),
  CONSTRAINT ck_onbpi_kind   CHECK (kind IN ('reading','video','meeting','practice','access_request')),
  CONSTRAINT ck_onbpi_status CHECK (status IN ('pending','in_progress','done','not_applicable')),
  CONSTRAINT fk_onbpi_plan   FOREIGN KEY (plan_id)               REFERENCES onboarding_plans(id) ON DELETE CASCADE,
  CONSTRAINT fk_onbpi_module FOREIGN KEY (source_module_id)      REFERENCES onboarding_modules(id),
  CONSTRAINT fk_onbpi_item   FOREIGN KEY (source_item_id)        REFERENCES onboarding_module_items(id),
  CONSTRAINT fk_onbpi_resp   FOREIGN KEY (responsible_person_id) REFERENCES people(id),
  CONSTRAINT fk_onbpi_user   FOREIGN KEY (status_changed_by)     REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS onboarding_share_links (
  id               BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  plan_id          BIGINT UNSIGNED NOT NULL,
  token_hash       BINARY(32) NOT NULL,                   -- sha256(token); o token só é exibido 1x
  token_hint       CHAR(4) NOT NULL,                      -- 4 últimos caracteres, p/ identificação
  expires_at       DATETIME(3) NOT NULL,
  revoked_at       DATETIME(3) NULL,
  revoked_by       BIGINT UNSIGNED NULL,
  created_by       BIGINT UNSIGNED NOT NULL,
  created_at       DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  last_accessed_at DATETIME(3) NULL,
  access_count     INT UNSIGNED NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY uq_onbl_token (token_hash),
  KEY ix_onbl_plan (plan_id),
  CONSTRAINT fk_onbl_plan    FOREIGN KEY (plan_id)    REFERENCES onboarding_plans(id),
  CONSTRAINT fk_onbl_revoker FOREIGN KEY (revoked_by) REFERENCES users(id),
  CONSTRAINT fk_onbl_creator FOREIGN KEY (created_by) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

São 44 tabelas mais `schema_migrations`.

### 3.10 Enums em código, settings e auditoria

**Enums fixos.** São servidos por `GET /api/v1/meta` com rótulos em pt-BR. Mudar um deles exige deploy.

| Enum | Valores |
|---|---|
| Senioridade | `jr, pl, sr, specialist, staff` |
| Perfil | `backend, frontend, fullstack, mobile, data, sre_platform, qa, security, management, other` |
| Idioma | `en, es, fr, de, it, other` |
| Nível de idioma | `basic, intermediate, advanced, fluent, native` |

Também ficam em código os tipos de entrevista, recomendação, origem, kinds de anexo, modalidade e prioridade.

**Settings.** Os defaults ficam em Go e cada chave é validada no `PUT`.

| Chave | Default | Efeito |
|---|---|---|
| `recruiting.retention_months` | 24 | `retention_expires_on = data_collected_on (ou renovação) + N meses` |
| `recruiting.retention_notice_days` | 30 | Antecedência do aviso |
| `recruiting.anonymize_grace_days` | 7 | Carência mínima entre o aviso e a anonimização |
| `recruiting.reopen_enabled` | false | Habilita reabrir Reprovado/Desistiu para o banco, só pelo Head |
| `lgpd.request_sla_days` | 15 | `due_on` dos pedidos do titular |
| `lgpd.access_log_retention_days` | 730 | Expurgo do `access_log` |
| `lgpd.processing_records` | JSON (ver abaixo) | Registro de tratamento exibido em `/admin/lgpd` |
| `onboarding.share_link_days_default` / `_max` | 30 / 90 | Expiração dos links |
| `ninebox.matrix` | JSON (ver abaixo) | Eixos, níveis, quadrantes e critérios. É copiado para `quarters.ninebox_matrix` ao abrir o quarter. |
| `ninebox.peer_policy` | `all_managers` | `all_managers`: snapshot automático. `manual`: começa vazio e o Head escolhe. |
| `ninebox.min_justification_chars` | 30 | Mínimo para submeter |
| `training.budget_scope` | `org_unit` | `org_unit` ou `team` (Q6) |
| `files.max_per_owner` | 20 | Anexos por candidato ou pleito |
| `feature.gupy` | false | RF-REC-09 |

O default de `lgpd.processing_records` é uma lista com finalidade, base legal, retenção e categorias, uma entrada para cada grupo: Candidatos, Dados restritos de candidato, Colaboradores (ERP), Nine Box, Pleitos de treinamento, Onboarding, Logs. Exemplo: Nine Box = execução de contrato / legítimo interesse, retenção "enquanto durar o vínculo + definição do Jurídico". Esse registro cumpre o RNF-06 "base legal por tipo de dado". O texto final vem do Jurídico.

**`ninebox.matrix` default.** Os rótulos são provisórios (Q8). x = Performance, y = Comportamento, `quadrant = (y-1)*3 + x`.

| y \ x | x=1 Baixo | x=2 Médio | x=3 Alto |
|---|---|---|---|
| **y=3 Alto** | 7 Engajado com baixa entrega | 8 Em crescimento | 9 Destaque |
| **y=2 Médio** | 4 Em desenvolvimento | 5 Consistente | 6 Entrega forte |
| **y=1 Baixo** | 1 Atenção | 2 Ajuste de postura | 3 Entrega com risco |

Formato do JSON: `{"xAxis":{"name","description","levels":[3]},"yAxis":{...},"quadrants":{"1":{"name","description","criteria"},...,"9":{...}}}`.

**Política de diff da auditoria.** Cada caso de uso grava `audit.Record(ctx, tx, Entry{Action, EntityType, EntityID, Subject, Before, After, Reason})`. `Before` e `After` são structs de snapshot, cujos campos têm a tag `audit:"plain"` ou `audit:"redacted"`.
- Um teste unitário percorre todos os tipos de snapshot e falha se algum campo estiver sem tag.
- Em runtime, um campo sem tag é tratado como `redacted`.

| Tipo | Campos |
|---|---|
| `plain` | ids, status (funil, pleito, quarter, registro), datas, custos, prioridade, orçamento, papéis, seniority/profile/source, legal_basis, owner, recomendação e scores de entrevista |
| `redacted` (vira `{"changed":true}`) | toda PII de candidato (nome, contatos, URLs, cidade, empresa, cargo, indicador, idiomas, notas, parecer), dados restritos e todo o conteúdo do Nine Box (níveis e textos) |

O "valor anterior" exigido pelo RNF-07 vem de três lugares:
- status do funil: `candidate_events` (`from_status`/`to_status`);
- status de pleitos: `training_transitions`;
- Nine Box: `ninebox_entry_events` (`before_json`/`after_json`), que segue a regra de visibilidade do registro.

---

## 4. API REST

### 4.1 Convenções

- **Versionamento:**
  - prefixo `/api/v1`; uma mudança incompatível cria `/api/v2`;
  - SPA e API saem juntas e toda resposta leva `X-App-Version: <sha>`. O SPA mostra "Nova versão disponível, recarregar" quando a versão muda.
- **Formato:**
  - JSON em camelCase;
  - `Content-Type: application/json` é obrigatório nas escritas (senão 415). Só as rotas de upload aceitam `multipart/form-data`.
  - corpo limitado a 1 MB (uploads até `FILES_MAX_BYTES` + 64 KB);
  - decoder com `DisallowUnknownFields`, para impedir mass assignment;
  - `DATE` como `YYYY-MM-DD`, instantes em RFC 3339 UTC, dinheiro em `*Cents` inteiro;
  - enums com chaves em inglês; os rótulos vêm de `/meta`.
- **Paginação:**
  - `?page=1&pageSize=25` (máximo 100), com resposta `{"items":[…],"page":1,"pageSize":25,"total":1234}`;
  - filtros multivalorados repetem o parâmetro (`status=a&status=b`);
  - `sort` só aceita valores de uma allowlist por endpoint, com prefixo `-` para ordem decrescente.
- **Erros:** RFC 9457 `application/problem+json`:
  `{"type":"urn:pp:error:<code>","title":"…","status":422,"code":"validation_failed","detail":"…pt-BR…","errors":{"email":"inválido"},"requestId":"…"}`.
  O 500 nunca traz detalhe, só o `requestId`.

  | Status | Códigos |
  |---|---|
  | 401 | `unauthenticated`, `session_expired`, `access_revoked` |
  | 403 | `forbidden` (papel, ou objeto visível com ação proibida), `csrf_failed` |
  | 404 | `not_found` (inclui objeto existente mas invisível, e rota atrás de flag desligada) |
  | 409 | `invalid_transition`, `stale_version`, `possible_duplicate`, `quarter_closed`, `quarter_not_open`, `anonymized`, `archived`, `pending_entries`, `pending_changed`, `batch_conflict`, `preview_outdated`, `already_exists` |
  | 413 | `payload_too_large` |
  | 415 | `unsupported_media_type` |
  | 422 | `validation_failed`, `malware_detected` |
  | 429 | `rate_limited` |
  | 503 | `unavailable` |
- **Concorrência:**
  - `PUT` exige `version` no corpo; se divergir, 409 `stale_version`;
  - transições usam `UPDATE … SET status=?, version=version+1 WHERE id=? AND status=?`. Se 0 linhas forem afetadas, 409 `invalid_transition`.
- **Transições:** `POST …/transitions {to, …}`.
- **Capacidades:** objetos de detalhe trazem `can:{…}` e `allowedTransitions:[…]`, calculados pelas mesmas funções de `policy.go`.
- **Cache:** `/api` e `/auth` sempre respondem `Cache-Control: no-store`.

### 4.2 Endpoints

**Papéis:**
- P = público.
- G = gestor. Toda sessão válida é G, porque o gate exige o papel.
- H = Head.
- A = Admin (sempre também gestor).
- A|H = qualquer um dos dois.

"obj:" indica a regra de objeto da §5. **As rotas das seções de núcleo a onboarding têm o prefixo `/api/v1`.**

**Infraestrutura, autenticação e público**

| Método | Rota | Papel | Descrição |
|---|---|---|---|
| GET | `/healthz` | P | Liveness, sem banco: `{"status":"ok","version":"<sha>"}`. É o check do Tsuru. |
| GET | `/readyz` | P | Ping no banco (1 s) e versão do schema. Usado por monitoração e E2E. |
| GET | `/metrics` | Bearer `METRICS_TOKEN` | Prometheus. Com token vazio, 404. |
| GET | `/auth/login?returnTo=` | P | Inicia Code + PKCE |
| GET | `/auth/callback` | P | Valida, aplica o gate e cria a sessão, ou redireciona para `/auth/denied` |
| POST | `/auth/logout` | G (CSRF) | Apaga a sessão e devolve `{redirectUrl}` |
| GET | `/auth/denied`, `/auth/logged-out` | P | Páginas HTML do servidor, sem dados |
| GET | `/s/onboarding` | P | Página estática (HTML + `share.js`). O token vai no fragmento `#t=…`. |
| POST | `/s/api/onboarding` | P + token, rate limit | `{token}` → visão mínima do plano. Inexistente, expirado ou revogado dão o mesmo 404. |
| POST | `/s/api/onboarding/export.md` | P + token, rate limit | Markdown como attachment |

**Núcleo**

| Método | Rota | Papel | Descrição |
|---|---|---|---|
| GET | `/me` | G | Usuário, papéis, personId, gerência, `managedTeamIds`, features, expirações da sessão, appVersion |
| GET | `/meta` | G | Enums e rótulos |
| GET | `/home` | G | Pendências por papel (§8) |
| GET | `/notifications?unread=` | G | Paginado |
| POST | `/notifications/{id}/read`, `/notifications/read-all` | G | obj: destinatário |
| GET | `/org-units/current`, `/teams` | G | |
| GET | `/people?q=&teamId=&active=` | G | Diretório e autocomplete |
| GET | `/people/{id}` | G | |
| GET | `/me/reports` | G | Liderados diretos efetivos |
| GET | `/me/teams` | G | ManagedTeams |
| GET | `/catalog/skills`, `/catalog/competencies`, `/catalog/job-titles` | G | Catálogos ativos |
| GET | `/quarters` | G | Com status e datas |
| POST | `/quarters` | A\|H | `{code}`. As datas saem do código e são editáveis. 409 `already_exists`. |
| PUT | `/quarters/{id}` | A\|H | Datas, prazo de treinamento e datas-alvo do Nine Box. 409 se `closed`. |

**Admin**

| Método | Rota | Papel | Descrição |
|---|---|---|---|
| POST, PUT | `/admin/catalog/{skills\|competencies\|job-titles}[/{id}]` | A | CRUD; desativar com `active:false` |
| POST | `/admin/erp-imports` | A | multipart CSV → lote `previewed` com diff e erros |
| GET | `/admin/erp-imports`, `/admin/erp-imports/{id}` | A | |
| POST | `/admin/erp-imports/{id}/apply` | A | `{confirmDeactivations}`. 409 `preview_outdated` se o diff mudou. A trava de 20% de desativações exige confirmação. |
| POST | `/admin/erp-imports/{id}/discard`, `/admin/erp-imports/{id}/reprocess` | A | O reprocess gera nova prévia a partir do mesmo arquivo |
| POST | `/admin/candidate-imports?dryRun=` | A | Carga inicial de planilhas (R4) |
| PUT, DELETE | `/admin/people/{id}/manager-override` | A | `{managerPersonId, reason}` (RF-AUTH-03) |
| PUT | `/admin/teams/{id}` | A | `managerPersonId`; nome, se `source=manual` |
| GET | `/admin/users` | A | |
| PUT | `/admin/users/{id}/person` | A | Vínculo manual |
| POST | `/admin/users/{id}/revoke-sessions`, `/disable`, `/enable` | A | |
| GET, PUT | `/admin/settings[/{key}]` | A | Validação por chave |
| GET | `/admin/audit?entityType=&entityId=&actorId=&action=&from=&to=` | A\|H | Só campos `plain` |
| GET | `/admin/access-log?subjectType=&subjectId=&userId=&from=&to=` | A\|H | |
| GET | `/admin/jobs` | A | Últimas execuções |
| POST | `/admin/jobs/{name}/run` | A | 202 `{runId}`, roda de forma assíncrona |
| POST | `/admin/integrations/gupy/sync` | A + `feature.gupy` | Flag desligada → 404 |
| GET | `/admin/success-metrics?from=&to=` | A\|H | PRD §9 |

**Recrutamento**

| Método | Rota | Papel | Descrição |
|---|---|---|---|
| GET | `/candidates` | G | Busca e filtros (abaixo). Resumo sem e-mail ou telefone. Exclui anonimizados. |
| GET | `/candidates/duplicates?email=&linkedin=` | G | `[{id, fullName, status, ownerName}]` |
| POST | `/candidates` | G | 409 `possible_duplicate {matches}` se não vier `confirmDuplicate:true` |
| GET | `/candidates/{id}` | G | Ficha sem restritos, com `can`/`allowedTransitions`. Faz upsert em `candidate_views`. |
| PUT | `/candidates/{id}` | G | `version`. Status, owner e campos LGPD não mudam por aqui. Inclui skills, tags, idiomas e vagas. |
| POST | `/candidates/{id}/transitions` | G (+H para reabrir) | `{to, note?, rejectionReason?, hire?:{teamId, jobTitleId?, startDate?}}` |
| POST | `/candidates/{id}/archive`, `/unarchive` | G, obj: owner ou H | |
| PUT | `/candidates/{id}/owner` | H | `{ownerUserId}` |
| GET | `/candidates/{id}/timeline` | G | |
| POST | `/candidates/{id}/notes` | G | `{body}` (evento `note`, imutável) |
| GET | `/candidates/{id}/restricted` | G, obj: entradas do autor; H vê todas | `access_log` quando retorna ≥ 1 entrada |
| POST | `/candidates/{id}/restricted` | G | `{kind, salaryCents?, body?}`; o autor é o ator |
| PUT, DELETE | `/restricted-entries/{id}` | obj: PUT só autor; DELETE autor ou H | |
| POST | `/candidates/{id}/consent-renewal` | G, obj: owner ou H | `{legalBasis, consentObtainedOn?, note}`; recalcula a expiração e zera `retention_warned_at` |
| POST | `/candidates/{id}/lgpd-requests` | G | `{kind, channel, receivedOn, details?}`; notifica Heads e owner |
| GET | `/lgpd-requests?status=` | H | |
| POST | `/lgpd-requests/{id}/resolve` | H | `{outcome:"done"\|"rejected", note}`. Erasure + done anonimiza na mesma tx. |
| GET | `/candidates/{id}/export.json` | H | Atende pedido de acesso; tudo, inclusive restritos. `access_log` `candidate.export`. |
| GET, POST | `/candidates/{id}/interviews` | G | POST `{interviewerPersonId?\|interviewerName, interviewedOn, kind, recommendation, opinion, scores:[{competencyId, score}]}` |
| PUT, DELETE | `/interviews/{id}` | obj: `created_by` ou H | |
| GET, POST | `/candidates/{id}/attachments` | G | POST multipart (`kind`, `file`) |
| GET | `/attachments/{id}/content` | obj: pode ver o dono | Download; `access_log` quando o dono é candidato |
| DELETE | `/attachments/{id}` | obj: quem enviou, owner do candidato ou H; pleito: requester ou H | |
| GET, POST | `/job-openings` | G | |
| PUT | `/job-openings/{id}` | G | |
| GET | `/tags?q=` | G | Autocomplete |

Filtros de `GET /candidates`:
- `q`: até 5 termos, cada um um `AND search_text LIKE ?`, com `% _ \` escapados.
- `skill=12:4`: repetível; cada ocorrência vira um `EXISTS` com `level >= 4`.
- `seniority`, `profile`, `status`, `source`, `jobTitleId`, `openingId`: repetíveis, viram `IN`.
- `tag`: repetível, combinado com AND.
- `createdFrom`/`createdTo`: dias em BRT convertidos para UTC.
- `ownerId=me`, `expiresWithinDays`, `includeArchived` (default false).
- `sort`: `-updatedAt` (default), `name`, `-createdAt`.

Os chips de skills e tags da página vêm de uma segunda query `IN (…)`, sem N+1.

**Treinamentos**

| Método | Rota | Papel | Descrição |
|---|---|---|---|
| GET | `/training/requests?quarterId=&teamId=&status=&personId=` | G, obj: escopo SQL (§5) | RF-TRN-05 |
| POST | `/training/requests` | G | Cria rascunho `{quarterId, teamId, title, …, beneficiaryIds}` |
| GET | `/training/requests/{id}` | G, obj: visível | Com transições, evidências, anexos e `can` |
| PUT | `/training/requests/{id}` | obj: requester e `draft` | `version` |
| DELETE | `/training/requests/{id}` | obj: requester e `draft` | |
| POST | `/training/requests/{id}/transitions` | obj: §6.2 | `{to, comment?, evidenceRef?, attachmentIds?}` |
| GET, POST | `/training/requests/{id}/attachments` | obj: ver (GET) / requester ou H (POST) | multipart |
| POST | `/training/decisions` | H | `{items:[{id, decision:"approve"\|"reject", comment?}]}` (máx. 100), atômico |
| GET | `/training/panel?quarterId=` | H | Por time: itens, totais, orçamento, comprometido, saldo, prioridade |
| GET | `/training/budgets?quarterId=` | A\|H | |
| PUT | `/training/budgets` | A\|H | `{quarterId, teamId?, amountCents}` (`teamId` só com scope `team`) |
| GET | `/training/export.csv?quarterId=` | H | `;`, BOM UTF-8, CRLF, proteção contra CSV injection |

**Nine Box**

| Método | Rota | Papel | Descrição |
|---|---|---|---|
| GET | `/ninebox/config` | G | Matriz vigente (para quarters `planned`) |
| POST | `/quarters/{id}/ninebox/open` | A\|H | `planned → open`; snapshot da matriz e dos participantes |
| POST | `/quarters/{id}/ninebox/start-calibration` | H | `open → calibration` |
| POST | `/quarters/{id}/ninebox/close` | H | `{confirmPending, expectedPendingCount}` |
| GET | `/quarters/{id}/ninebox/participants` | G | |
| PUT | `/quarters/{id}/ninebox/participants` | H | `{personIds}`, só em `open`/`calibration` |
| GET | `/quarters/{id}/ninebox/matrix?teamId=&managerPersonId=` | G, filtrado por `CanView` | Contagens e nomes por quadrante (só submetidos visíveis); `access_log` |
| GET | `/quarters/{id}/ninebox/entries?teamId=&managerPersonId=&quadrant=&status=` | G, filtrado por `CanView` | `access_log` |
| GET | `/quarters/{id}/ninebox/my-reports` | G | Liderados diretos e status do registro |
| GET | `/quarters/{id}/ninebox/coverage` | G (próprios) / H (todos) | RF-9BOX-09; só nomes e status |
| GET | `/quarters/{id}/ninebox/movement` | G, filtrado | RF-9BOX-07; `access_log` |
| POST | `/ninebox/entries` | obj: gestor direto atual, quarter `open`/`calibration` | `{quarterId, personId, …}`; 409 `already_exists {id}` |
| GET | `/ninebox/entries/{id}` | obj: `CanView` | Registro, eventos e comentários, com `can`; `access_log` |
| PUT | `/ninebox/entries/{id}` | obj: `CanEdit` ou `CanMediate` | `{version, xLevel, yLevel, justification, strengths, developmentPoints, agreedActions, reason?}` |
| POST | `/ninebox/entries/{id}/submit` | obj: `CanEdit` | |
| POST | `/ninebox/entries/{id}/comments` | obj: `CanComment` | `{body ≤ 4000}` |
| POST | `/ninebox/entries/{id}/rectify` | H, quarter `closed` | `{version, campos, reason}` |
| GET | `/ninebox/people/{personId}/history` | G, filtrado por `CanView` | RF-9BOX-05; `access_log` |

**Onboarding**

| Método | Rota | Papel | Descrição |
|---|---|---|---|
| GET | `/onboarding/modules?scope=&teamId=&includeArchived=` | G | Catálogo, com `can.edit` por módulo |
| POST | `/onboarding/modules` | obj: common = A\|H; team = A\|H ou G com o time em ManagedTeams | Cria a versão 1 e sua revisão |
| GET, PUT | `/onboarding/modules/{id}` | G / obj edição | O PUT gera nova revisão |
| POST | `/onboarding/modules/{id}/archive`, `/unarchive` | obj edição | |
| PUT | `/onboarding/module-order` | obj edição | `{scope, teamId?, ids}` |
| POST | `/onboarding/modules/{id}/items` | obj edição | |
| PUT, DELETE | `/onboarding/module-items/{id}` | obj edição | DELETE arquiva o item |
| PUT | `/onboarding/modules/{id}/item-order` | obj edição | `{ids}` |
| GET | `/onboarding/modules/{id}/revisions[/{version}]` | G | Histórico e snapshot |
| GET | `/onboarding/plans?status=&teamId=` | G (próprios) / H (todos) | Com % de progresso e atrasados |
| POST | `/onboarding/plans/preview` | G | `{teamId, startDate, moduleIds?}` → itens calculados, sem gravar |
| POST | `/onboarding/plans` | obj: H ou time em ManagedTeams | `{onboardeeName, onboardeeEmail?, personId?, jobTitleId?, teamId, startDate, buddyPersonId?, moduleIds?, composerStartedAt?}` → plano `active` |
| GET, PUT | `/onboarding/plans/{id}` | obj: responsável ou H | PUT com `recalculateDueDates?`; só o H troca `managerUserId` |
| POST | `/onboarding/plans/{id}/activate` | obj | `draft → active {startDate, moduleIds?}`; gera os itens |
| POST | `/onboarding/plans/{id}/items` | obj | Item avulso |
| PUT, DELETE | `/onboarding/plan-items/{id}` | obj | Edição e status do checklist |
| PUT | `/onboarding/plans/{id}/item-order` | obj | |
| POST | `/onboarding/plans/{id}/complete`, `/cancel` | obj | O cancel exige `{reason}` |
| GET | `/onboarding/plans/{id}/export.md` | obj | |
| GET, POST | `/onboarding/plans/{id}/share-links` | obj | POST `{expiresInDays}` → `{id, url, expiresAt}`; o token é mostrado **uma vez** |
| POST | `/onboarding/share-links/{id}/revoke` | obj | |
| GET | `/onboarding/dashboard` | G (próprios) / H | RF-ONB-06 |

---

## 5. Matriz de autorização

### 5.1 Papéis e portão de entrada (RF-AUTH-01/02)

**Mapeamento:**
- Os grupos vêm da claim `OIDC_GROUPS_CLAIM`. Ela aceita caminho com ponto (ex.: `realm_access.roles`) e valor em array ou string separada por espaço ou vírgula.
- A comparação é exata, depois de remover uma `/` inicial dos dois lados. Assim `/gestores` (path de grupo do Keycloak) casa com `gestores`.
- `ROLE_GROUPS_MANAGER`, `ROLE_GROUPS_HEAD` e `ROLE_GROUPS_ADMIN` são CSV; no Azure AD, são GUIDs.

**Regras:**
- **Head ⇒ Manager.**
- **Gate: o papel `manager` é obrigatório.** Sem ele, não se cria sessão nem linha em `users`, e o usuário vai para `/auth/denied`.
- **Admin é papel aditivo:** um usuário só com o grupo admin é negado.
- Overage do Azure (`_claim_names` contendo a claim de grupos) é negado e registrado em log.
- O gate é reavaliado a cada recheck (§7). Perder o gate revoga a sessão.

### 5.2 Matriz

"—" = não pode.

| Recurso / ação | Gestor | Head | Admin (+Gestor) |
|---|---|---|---|
| Candidato: listar, ver ficha (sem restritos), criar, editar, mover no funil, notas, entrevistas, anexos, vagas | sim (banco compartilhado da gerência, RF-REC-07) | sim | como gestor |
| Candidato anonimizado | some da busca; a ficha vira um stub sem PII; qualquer escrita retorna 409 `anonymized` | idem | idem |
| Arquivar/desarquivar, renovar consentimento | só o owner (`owner_user_id`) | sim | como gestor |
| Transferir responsável (owner) | — | sim | — |
| Dados restritos: criar entrada | sim, e o autor é quem cria | sim | como gestor |
| Dados restritos: ler | só as entradas de que é autor | todas | como gestor |
| Dados restritos: editar | só a própria entrada | só a própria | como gestor |
| Dados restritos: apagar | só a própria entrada | qualquer | como gestor |
| Entrevista: editar ou excluir | quem registrou (`created_by`) | sim | como gestor |
| Anexo de candidato: baixar | sim (gera `access_log`) | sim | como gestor |
| Anexo de candidato: remover | quem enviou ou o owner | sim | como gestor |
| Pedido LGPD: registrar | sim | sim | como gestor |
| Pedido LGPD: resolver, anonimizar, exportar JSON | — | sim | — |
| Catálogo de onboarding comum: editar | — | sim | sim |
| Catálogo de onboarding de time: editar | só time em ManagedTeams | sim | sim |
| Catálogo de onboarding: ver, incluindo revisões | sim | sim | sim |
| Plano de onboarding: criar | só time em ManagedTeams | sim | como gestor |
| Plano: ver, editar, checklist, links, exportar, concluir, cancelar | responsável (`manager_user_id`) | todos (inclui reatribuir) | como gestor |
| Quarters: criar, editar datas, abrir Nine Box | — | sim | sim |
| Nine Box: iniciar calibração, editar participantes, fechar, retificar | — | sim | — |
| Nine Box: criar, editar, submeter registro | gestor direto atual, com quarter `open` ou `calibration` | idem, para os liderados diretos dele | como gestor |
| Nine Box: mediar (ajustar registro submetido de terceiros) | — | sim, durante `calibration` | — |
| Nine Box: ver registro, matriz, histórico, movimentação | regra `CanView` (§5.3) | todos, inclusive rascunhos | como gestor |
| Nine Box: comentar | quem vê o registro, com quarter em `calibration` e registro submetido | sim | como gestor |
| Nine Box: cobertura | só os próprios liderados | todos os gestores | como gestor |
| Pleito: criar, editar rascunho, submeter, recolher, excluir rascunho | requester; time em ManagedTeams; beneficiários precisam ser liderados diretos (revalidado ao submeter) | qualquer time ou pessoa ativa | como gestor |
| Pleito: ver | requester, ou gestor atual de algum beneficiário (exceto rascunho) | todos os não-rascunho, mais os próprios | como gestor |
| Pleito: aprovar/reprovar (individual ou em lote) | — | sim | — |
| Pleito: pago, em andamento, concluído, cancelado | requester | sim | como gestor |
| Orçamento | — | sim | sim |
| Painel trimestral, export CSV e impressão | — | sim | — |
| Auditoria e log de acesso | — | sim | sim |
| Métricas de sucesso | — | sim | sim |
| Catálogos (skills, competências, cargos), settings e flags, matriz do Nine Box, import ERP e de candidatos, overrides, times, usuários, jobs, Gupy | — | — (salvo se também for Admin) | sim |

### 5.3 Regras de visibilidade

**Nine Box (RF-9BOX-08)** fica em `internal/ninebox/policy.go`. São funções puras sobre fatos já carregados:

```go
func CanView(p auth.Principal, e EntryFacts, q QuarterFacts, s SubjectFacts) bool {
    if s.PersonID == p.PersonID || q.OrgUnitID != p.OrgUnitID { return false } // ninguém vê o próprio registro
    if p.IsHead() { return true }                                             // Head vê todos (inclui rascunho)
    if e.AuthorUserID == p.UserID { return true }                             // quem escreveu
    if p.PersonID != 0 && s.EffectiveManagerPersonID == p.PersonID { return true } // gestor direto ATUAL (D3)
    // pares: participantes da calibração, durante e após a calibração, só submetidos
    return e.Status == Submitted &&
        (q.Status == Calibration || q.Status == Closed) &&
        q.IsParticipant(p.PersonID) &&
        !q.IsParticipant(s.PersonID) // registro de quem também é participante (gestores, avaliados pelo Head) não é exposto aos pares
}
func CanEdit(p, e, q, s) bool    { return (q.Status == Open || q.Status == Calibration) && p.PersonID != 0 && s.EffectiveManagerPersonID == p.PersonID && s.Active }
func CanMediate(p, e, q, s) bool { return p.IsHead() && q.Status == Calibration && e.Status == Submitted && s.PersonID != p.PersonID }
func CanComment(p, e, q, s) bool { return CanView(p, e, q, s) && q.Status == Calibration && e.Status == Submitted }
func CanRectify(p, e, q, s) bool { return p.IsHead() && q.Status == Closed && s.PersonID != p.PersonID }
```

"Pares" são os pares do **avaliador**. Os gestores avaliam devs e o Head avalia os gestores. Como o Head não tem pares na gerência, os registros dos gestores ficam visíveis só ao Head, ao gestor direto atual e a quem os escreveu.

**Dados restritos.** O predicado SQL é `author_user_id = :user OR :isHead`. Os restritos nunca entram no DTO da ficha nem em JOINs gerais.

**Pleitos.** O predicado SQL é:

```sql
t.org_unit_id = :org AND (t.requester_user_id = :user OR (t.status <> 'draft' AND (:isHead OR EXISTS (
  SELECT 1 FROM training_request_beneficiaries b JOIN people pe ON pe.id = b.person_id
  WHERE b.request_id = t.id AND pe.effective_manager_person_id = :person))))
```

**Planos.** O predicado SQL é `p.org_unit_id = :org AND (:isHead OR p.manager_user_id = :user)`.

**ManagedTeams(p).** Usado para criar pleitos e planos e para editar trilhas de time:

```
{t.id : t.manager_person_id = p.PersonID} ∪ {pe.team_id : pe.effective_manager_person_id = p.PersonID AND pe.active}
```

### 5.4 Onde a autorização é aplicada no código

1. **Gate**, em `internal/auth`. É aplicado no callback e em cada recheck. Sessões só existem para quem tem o papel `manager`.
2. **Portão de papel**, em `internal/server/routes.go`. É uma tabela única, por exemplo:
   ```go
   r.api("GET /api/v1/candidates", auth.G, rec.List)
   r.api("POST /api/v1/training/decisions", auth.H, trn.Decide)
   r.api("POST /api/v1/quarters", auth.A|auth.H, qtr.Create)
   r.public("GET /healthz", health.Live)
   ```
   - `r.api` exige um `RoleSet` diferente de zero e dá panic sem ele.
   - `r.public` só aceita padrões de uma allowlist fixa.
   - O teste `TestEveryRouteGated` percorre o registro e falha se houver rota sem portão ou rota pública fora da allowlist.
3. **Política por objeto**, no `policy.go` de cada módulo. O método do módulo carrega a linha **dentro da tx**, chama a política e, conforme o caso:
   - devolve `NotFound` se o objeto é invisível ao ator;
   - devolve `Forbidden` se o objeto é visível mas a ação é proibida.

   As mesmas funções preenchem `can` e `allowedTransitions`.
4. **Listas:**
   - Nine Box: o volume por quarter tem centenas de linhas, então o módulo carrega e **filtra em Go com o próprio `CanView`**. Fica uma única fonte de verdade.
   - Pleitos, planos e restritos: usam os predicados SQL da §5.3. O teste de equivalência confere, para cada ator do dataset, que o conjunto do SQL é igual ao conjunto permitido pela política.
   - Candidatos: `org_unit_id = :org AND anonymized_at IS NULL`.
5. **DTOs.** Cada caso de uso tem seu struct de resposta. Testes de integração verificam a **ausência das chaves** (ex.: `salaryCents` nunca aparece em `GET /candidates/{id}`).
6. **`access_log`.** `audit.Access(ctx, tx, user, action, resource, subjects…)` é chamado dentro do método de leitura sensível, antes de serializar. É **fail-closed**: se a gravação falhar, a resposta é 500 e o dado não sai.
7. **Frontend.** Papéis e `can` servem só à experiência do usuário. O backend é a barreira.

---

## 6. Máquinas de estado

As tabelas de transição vivem em `model.go` como funções puras, testadas por enumeração exaustiva de (de, para, ator). Toda transição roda numa única tx, que contém:
- o CAS em status e version;
- o evento do agregado;
- o `audit_log` com from/to em `plain`;
- a notificação, quando houver.

### 6.1 Funil do candidato (RF-REC-04)

| De | Para | Quem | Exige / efeito |
|---|---|---|---|
| `talent_pool` | `in_process` | G | `note` opcional |
| `in_process` | `approved` | G | |
| `in_process` | `talent_pool` | G | `note` obrigatória (processo pausado) |
| `approved` | `in_process` | G | `note` obrigatória |
| `approved` | `hired` | G | `hire.teamId` obrigatório. Cria `onboarding_plans` em `draft` com nome, e-mail, cargo, time, `candidate_id` e responsável = ator (RF-REC-08, a partir do M8). |
| `talent_pool`, `in_process`, `approved` | `rejected` | G | `rejectionReason` obrigatório, gravado como entrada restrita `rejection_reason` (autor = ator); `note` pública opcional |
| `talent_pool`, `in_process`, `approved` | `withdrawn` | G | `note` opcional |
| `rejected`, `withdrawn` | `talent_pool` | H | Só com `recruiting.reopen_enabled=true` (default **false**). `note` obrigatória. |
| `hired` | — | — | Terminal |

- Candidato arquivado: nenhuma transição até desarquivar (409 `archived`).
- Candidato anonimizado: 409 `anonymized`.
- Arquivar não muda o status.

### 6.2 Pleito de treinamento (RF-TRN-04)

| De | Para | Quem | Exige |
|---|---|---|---|
| (criação) | `draft` | G | `title`, `teamId`, `quarterId` (vigente ou próximo) |
| `draft` | `submitted` | requester | `provider`, `modality`, `content`, `justification`; ≥ 1 beneficiário; time em ManagedTeams e beneficiários entre os liderados diretos (exceto para H); quarter ainda vigente ou próximo |
| `submitted` | `draft` | requester | Recolher antes da decisão |
| `submitted` | `approved` | H (individual ou lote) | `comment` opcional. Não bloqueia por orçamento; a resposta traz `overBudget:true` e a UI confirma. |
| `submitted` | `rejected` | H | `comment` obrigatório. Terminal. |
| `approved` | `paid` | requester, H | Evidência: ≥ 1 anexo `invoice`/`payment_proof` **ou** `evidenceRef`. Dispensada se `cost_total_cents = 0`. |
| `paid` | `in_progress` | requester, H | |
| `in_progress` | `completed` | requester, H | Anexo `certificate`, `evidenceRef` ou `comment` ≥ 10 caracteres. Terminal. |
| `approved`, `paid`, `in_progress` | `cancelled` | requester, H | `comment` (motivo) obrigatório. Terminal. |

- **Vigente e próximo:** vigente é o quarter da gerência com `starts_on ≤ hoje(BRT) ≤ ends_on`; próximo é o de menor `starts_on > hoje`.
- **Edição:** livre só em `draft`. Depois disso, só é possível anexar.
- **Lote:** uma tx com `SELECT … WHERE id IN (…) AND org_unit_id=? FOR UPDATE`.
  - Qualquer item fora de `submitted`, invisível ou com reprovação sem comentário causa rollback e 409 `batch_conflict {errors:{id: code}}`.
  - Todos os itens do lote compartilham o `batch_id`.
  - A tx notifica cada requester (RF-TRN-06).
- **Painel:** "comprometido" é a soma de `approved`, `paid`, `in_progress` e `completed`. Itens `rejected` e `cancelled` ficam fora dos totais.

### 6.3 Ciclo do quarter (Nine Box, RF-9BOX-01/04)

```
planned ─(A|H: open)─► open ─(H: start-calibration)─► calibration ─(H: close)─► closed
```

| Transição | Efeitos |
|---|---|
| `open` | Exige não haver outro quarter `open`/`calibration` na gerência (`uq_quarters_live`). Copia `ninebox.matrix` para `ninebox_matrix`. Com `peer_policy=all_managers`, insere participantes: os `effective_manager_person_id` distintos de pessoas ativas da gerência, mais as pessoas vinculadas a usuários com papel manager/head. Notifica os gestores com usuário. |
| `start-calibration` | Registros submetidos passam a ser visíveis aos pares |
| `close` | Calcula as pendências: rascunhos mais liderados ativos sem registro. Se houver pendência e `confirmPending=false`, responde 409 `pending_entries {list}`. Se `expectedPendingCount` divergir da contagem real, 409 `pending_changed`. Grava `close_entries_submitted`/`close_people_total`. Rascunhos ficam congelados e fora da matriz. |

- As transições são manuais. As datas-alvo só geram avisos na home. Não há volta de estado.
- Datas podem ser editadas até `closed`.
- **Corrida entre edição e fechamento:** toda escrita em registro faz `SELECT ninebox_status FROM quarters WHERE id=? FOR SHARE` no início da tx, e o fechamento faz `… FOR UPDATE`.

### 6.4 Registro do Nine Box

| Ação | Condição | Efeito |
|---|---|---|
| Criar ou editar rascunho | `CanEdit` | Campos parciais permitidos; auditoria com níveis e textos `redacted` |
| `draft → submitted` | `CanEdit`; x, y e justificativa ≥ `min_justification_chars` | Evento `submitted`. `manager_person_id` e `team_id` recebem snapshot. |
| Ajuste após submissão | `CanEdit`, até o fechamento | `reason` ≥ 10 caracteres; evento `adjusted` com before/after |
| Mediação do Head | `CanMediate` | `reason` obrigatório; evento `calibrated`; notifica o gestor direto |
| Retificação | `CanRectify` | `reason` obrigatório; evento `rectified`; status passa a `submitted`; grava `last_rectified_at` |

### 6.5 Demais estados

- **Plano de onboarding:**
  - `draft` (pré-cadastro vindo da contratação) → `active` (via `activate`) → `completed`;
  - `draft`/`active` → `cancelled`, com motivo.
- **Item do plano:** transita livremente entre `pending`, `in_progress`, `done` e `not_applicable`.
  - Progresso = `done / (total − not_applicable)`, ou 0 quando não há itens elegíveis.
  - Atrasado = `due_date < hoje` e status em `pending`/`in_progress`.
- **Link de compartilhamento:**
  - ativo quando `revoked_at IS NULL AND expires_at > now`;
  - expirado ou revogado responde o mesmo 404.
- **Pedido LGPD:** `open → done | rejected`, decidido pelo H, com `note`. Pedido vencido gera notificação diária aos Heads.

---

## 7. Fluxo de autenticação (BFF OIDC)

1. **Entrada.**
   - HTML do SPA sem sessão válida: o servidor responde 302 para `/auth/login?returnTo=<path+query>`. Se a consulta ao banco falhar, a resposta é uma página 503, sem loop.
   - `/api` sem sessão responde 401. O SPA mostra "Sua sessão expirou" com o botão "Entrar novamente", sem redirecionar sozinho.
2. **`GET /auth/login`** (rate limit de 30/min por IP).
   - **Validação de `returnTo`**, já decodificado:
     - rejeita qualquer byte `< 0x20`, `0x7f` ou `\`;
     - faz `url.Parse` e exige `Scheme == ""` e `Host == ""`;
     - exige path começando com `/`, mas não com `//`, `/auth` ou `/api`;
     - em qualquer falha, usa `/`;
     - reserializa com `u.RequestURI()`.
   - Gera `state` e `nonce` (32 B base64url cada) e `verifier = oauth2.GenerateVerifier()`.
   - Grava o cookie `__Host-pp_oidc` = `seal({state, nonce, verifier, returnTo, iat}, AAD "oidc_tx")`, com `HttpOnly; Secure; SameSite=Lax; Path=/; Max-Age=600`.
   - Responde 302 para `AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier))`.
3. **`GET /auth/callback`**:
   1. Se veio `?error=`, loga `auth.idp_error` e redireciona para `/auth/denied`.
   2. Abre o cookie, apaga-o, exige `iat` ≤ 10 min e compara `state` em tempo constante.
   3. Se vier o parâmetro `iss`, ele precisa ser igual ao issuer (RFC 9207).
   4. `Exchange` com `VerifierOption`, timeout de 10 s e autenticação `client_secret_basic`. Com segredo vazio, é cliente público com PKCE.
   5. Verifica o id_token com go-oidc: `SupportedSigningAlgs` RS256/PS256/ES256, `iss`, `aud`, `exp` com tolerância de 1 min. Confere o `nonce`.
   6. Lê `sub`, e-mail (`OIDC_EMAIL_CLAIM`), nome (`OIDC_NAME_CLAIM`) e grupos (`OIDC_GROUPS_CLAIM`).
      - Grupos ausentes no id_token: faz um único fallback para `UserInfo`.
      - `_claim_names` com a claim de grupos (overage): nega.
   7. Mapeia papéis e aplica o gate. Se negar: audit `auth.denied` com `reason = "sub_sha256=<hex>"`, sem sessão, 303 para `/auth/denied`.
   8. Faz upsert de `users` por `(iss, sub)`. Se `disabled_at` estiver preenchido, nega. Grava `last_roles` e `last_login_at`.
      - Se `person_id` for NULL, vincula pela claim `OIDC_EMPLOYEE_ID_CLAIM` (= `employee_number`, `person_link='employee'`), senão por `email_norm` (`'email'`).
   9. Cria a sessão:
      - revoga a sessão anterior, se o cookie existir;
      - token = 32 B aleatórios; `sessions.token_hash = sha256`;
      - `refresh_token_enc` e `id_token_enc` com `seal`;
      - `roles_checked_at = now`;
      - `absolute_expires_at = now + SESSION_ABSOLUTE_TIMEOUT` (12 h), ou `+ SESSION_NO_REFRESH_MAX` (4 h) se o IdP não devolveu refresh token.
   10. Grava o cookie `__Host-pp_sid` (`HttpOnly; Secure; SameSite=Lax; Path=/`, sem Max-Age), o audit `auth.login`, e responde 303 para `returnTo`.
4. **A cada request** (`session.Require`):
   - calcula o sha256 do cookie e faz um SELECT de `sessions ⨝ users ⟕ people`;
   - exige `absolute_expires_at > now`, `last_seen_at + SESSION_IDLE_TIMEOUT (2 h) > now` e `disabled_at IS NULL`;
   - atualiza `last_seen_at` no máximo a cada 60 s e faz upsert de `user_activity_days`;
   - monta o `Principal`; `OrgUnitID` = gerência da pessoa ou `DEFAULT_ORG_UNIT_CODE`.
5. **Recheck de papéis** (RNF-02, Backstage). Ocorre quando `roles_checked_at + ROLE_RECHECK_INTERVAL (15 min) < now` e existe refresh token:
   1. Abre uma tx: `SELECT roles_checked_at, refresh_token_enc FROM sessions WHERE token_hash=? FOR UPDATE NOWAIT`.
      - Lock ocupado (erro 3572), ou seja, outra unidade ou request já está renovando: rollback e segue com os papéis atuais.
      - `roles_checked_at` ficou recente: commit, recarrega e segue.
   2. Faz o refresh grant (timeout 5 s). Lê os grupos do novo id_token ou, se ele não vier, do userinfo com o novo access token. Remapeia e reaplica o gate.
   3. Conforme o resultado:
      - Gate ok: `UPDATE` de `roles`, `refresh_token_enc` (se rotacionado), `id_token_enc`, `roles_checked_at` e `recheck_failing_since=NULL`. **O ID da sessão não é rotacionado**, para não gerar 401 em cascata.
      - Gate perdido, ou `invalid_grant`: `DELETE` da sessão, audit `auth.roles_lost`, 401 `access_revoked`.
      - Rede ou 5xx: grava `recheck_failing_since` se ainda for NULL. Se `now − recheck_failing_since > ROLE_RECHECK_GRACE (1 h)`, apaga a sessão e responde 401 `session_expired`; senão segue.
   4. Commit. O row lock dura no máximo o timeout de 5 s.
6. **Logout** (`POST /auth/logout`, com CSRF):
   - apaga a sessão e responde `Set-Cookie` com `Max-Age=0`;
   - devolve `{redirectUrl}` = `end_session_endpoint` (do discovery) com `id_token_hint`, `post_logout_redirect_uri=APP_BASE_URL/auth/logged-out` e `client_id`; sem esse endpoint, `/auth/logged-out`;
   - o SPA faz `queryClient.clear()` e `location.assign`;
   - a página "Você saiu" evita o re-login automático pelo SSO.
7. **CSRF**, para POST/PUT/PATCH/DELETE em `/api` e `/auth/logout`, sem token sincronizador:
   - `Sec-Fetch-Site` presente e diferente de `same-origin` → 403 `csrf_failed`;
   - senão, `Origin` precisa ser igual à origem de `APP_BASE_URL`;
   - sem nenhum dos dois headers → 403;
   - escrita JSON exige `Content-Type: application/json`;
   - não há CORS: nenhum header `Access-Control-*`, e o preflight responde 405;
   - o `SameSite=Lax` cobre o restante.
8. **Headers de segurança:**

   | Classe | Headers |
   |---|---|
   | Todas | `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: strict-origin-when-cross-origin`, `Permissions-Policy: camera=(), microphone=(), geolocation=(), payment=()`, `Cross-Origin-Opener-Policy: same-origin`; HSTS `max-age=31536000` só quando `APP_BASE_URL` é https |
   | SPA | CSP `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'`. `style 'unsafe-inline'` é exigência do Mantine. Assets: `public, max-age=31536000, immutable`; `index.html`: `no-cache`. |
   | API | `default-src 'none'; frame-ancestors 'none'` |
   | Download | `default-src 'none'; sandbox`, `Content-Disposition: attachment; filename*=UTF-8''…`, `Cache-Control: private, no-store` |
   | `/s/*` | CSP `default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'`, `Referrer-Policy: no-referrer`, `X-Robots-Tag: noindex, nofollow`, `Cache-Control: no-store` |

9. **Modo dev e teste** (`internal/devidp` + `cmd/devidp`, estado em memória, chave RSA gerada na partida):
   - **Endpoints:**
     - `/.well-known/openid-configuration` e `/jwks`;
     - `/authorize`: página com um botão por persona. Valida `client_id`, `redirect_uri` exata e `code_challenge_method=S256`;
     - `/token`: grants `authorization_code` e `refresh_token`. Valida PKCE, `redirect_uri` e client secret. Rotaciona o refresh token e invalida o anterior;
     - `/userinfo`, `/logout` (redireciona para `post_logout_redirect_uri`);
     - `PUT /_admin/personas/{login}/groups` e `POST /_admin/personas/{login}/revoke`, usados no teste de perda de papel e de `invalid_grant`.
   - **Variantes negativas**, por flag de persona em testes: `bad_nonce`, `bad_iss`, `alg_none`, `expired`, `overage`, `groups_only_in_userinfo`, `nested_claim`.
   - **Configuração:** personas em `dev/personas.json`; clientes em `dev/clients.json` (`processos-dev` / `dev-secret`; redirect URIs `http://localhost:5173/auth/callback` e `http://localhost:8080/auth/callback`).

   | Login | Grupos | Resultado |
   |---|---|---|
   | `helena.head` | gestores, head | Head |
   | `ana.gestora` | gestores | Gestora do time Pagamentos |
   | `bruno.gestor` | gestores | Gestor do time Catálogo |
   | `carla.gestora` | gestores | Gestora do time Plataforma |
   | `alice.admin` | gestores, admins | Admin (+ gestora do time Dados) |
   | `fabio.soadmin` | admins | **Negado** |
   | `eva.semgrupo` | — | **Negado** |

   - **Cookies em dev/test:** `COOKIE_SECURE=false` só é aceito com `APP_ENV ∈ {dev,test}`. Nesse modo os cookies perdem `Secure` e o prefixo `__Host-` (viram `pp_sid` e `pp_oidc`). Isso faz funcionar `cookiejar` sobre `httptest` (http) e o Chromium em `http://localhost`.
   - **Travas de produção** (`config.Validate`, também via `server config check`): com `APP_ENV=production`, o server recusa subir se:
     - `APP_BASE_URL` ou `OIDC_ISSUER_URL` não forem https, ou o issuer for localhost;
     - `COOKIE_SECURE=false`;
     - `SESSION_ENC_KEY` tiver menos de 32 B;
     - `ROLE_GROUPS_MANAGER` estiver vazio;
     - `DEFAULT_ORG_UNIT_*` estiver ausente.
   - **Isolamento do mock:** `make check-prod-deps` falha se `go list -deps ./cmd/server` contiver `internal/devidp` ou `internal/devseed`.

---

## 8. Frontend

### 8.1 Estrutura de `web/src`

- **`main.tsx`:** MantineProvider (tema, contraste AA, `lang="pt-BR"`), DatesProvider `pt-br`, Notifications, QueryClientProvider e RouterProvider.
- **`router.tsx`:** rotas lazy; cada rota declara `handle.roles`.
- **`api/client.ts`:** `fetch` com `credentials:'same-origin'` e JSON. Converte problem+json em `ApiError`. Trata 401 (`session_expired`/`access_revoked`), compara `X-App-Version` e expõe `upload()` para multipart.
- **`api/types.ts`:** tipos escritos à mão. **`api/queries/*.ts`:** hooks por módulo (`useCandidates`, `useTransitionCandidate`, `useNineBoxMatrix`…).
- **`auth/`:** `useMe()` (`staleTime` 5 min), `<RequireRole roles>` (mostra "Sem permissão") e `SessionExpiredModal`.
- **`layout/AppLayout.tsx`:** AppShell com navegação filtrada por papel, sino de notificações (badge com não lidas, polling de 60 s) e menu do usuário com "Sair".
- **`components/`:**

  | Componente | Função |
  |---|---|
  | `DataTable` | Mantine Table + Pagination no servidor |
  | `FilterBar` | Sincronizada com a URL |
  | `PageHeader`, `StatusBadge`, `EmptyState`, `ConfirmModal` | Básicos de layout e estado |
  | `EventTimeline` | Linha do tempo de eventos |
  | `AttachmentList` + `FileDropzone` | Pré-checagem de tipo e tamanho; o servidor decide |
  | `PersonSelect` | Busca assíncrona em `/people` |
  | `MoneyInput` | Reais ↔ centavos, `Intl.NumberFormat('pt-BR', {style:'currency', currency:'BRL'})` |
  | `TransitionButtons` | Renderiza `allowedTransitions` e abre um modal com os campos exigidos por transição |
  | `StaleVersionModal` | Tratamento do 409 `stale_version` |
  | `SensitiveDataNotice` | "Acesso registrado — LGPD" |
- **`lib/`:** `format.ts` (datas `dd/MM/yyyy`, BRL), `qs.ts` (filtros ↔ URL) e `dates.ts` (prévia de D+n).
- **`features/{home,candidates,training,ninebox,onboarding,people,admin,notifications}/`**.

### 8.2 Rotas e páginas

| Rota | Página / componentes principais | Papel |
|---|---|---|
| `/` | Início com cards de pendência vindos de `/home` (ver abaixo) | G |
| `/candidatos` | Lista + `CandidateFilters`: texto, linhas "stack ≥ nível", multiselects, tags, período, "meus", "expira em 30 dias" | G |
| `/candidatos/novo`, `/candidatos/:id/editar` | `CandidateForm` com `StackSkillEditor` (catálogo + `Rating` 1–5 + anos), idiomas, tags, vagas, origem/indicação, bloco LGPD; `DuplicateAlert` ao sair do campo de e-mail ou LinkedIn | G |
| `/candidatos/:id` | Abas Ficha · Entrevistas (`ScorecardEditor`) · Anexos · Linha do tempo · Dados restritos · LGPD | G |
| `/vagas` | Vagas | G |
| `/lgpd/solicitacoes` | Pedidos do titular, prazo, resolver | H |
| `/treinamentos`, `/treinamentos/novo`, `/treinamentos/:id`, `/treinamentos/:id/editar` | Lista (quarter, time, status, pessoa); `TrainingForm` com custos e total ao vivo; detalhe com transições, evidências e anexos | G |
| `/treinamentos/painel?quarterId=` | Agrupado por time, checkboxes, `BatchDecisionBar` (total selecionado, saldo projetado, aprovar/reprovar com comentário), `BudgetSummaryCard`, CSV | H |
| `/treinamentos/painel/:quarterId/impressao` | Versão para impressão ("Salvar como PDF", RF-TRN-07) | H |
| `/treinamentos/orcamento` | Por gerência ou por time, conforme a setting | A\|H |
| `/quarters` | Criar, editar datas e prazo, abrir, iniciar calibração, fechar (`CloseQuarterModal` com a lista de pendências) | A\|H |
| `/nine-box` | Redireciona para o quarter vivo ou o mais recente | G |
| `/nine-box/:quarterId` | Abas Matriz (`NineBoxGrid`) · Registros · Cobertura · Movimentação · Participantes; filtros de time e gestor | G |
| `/nine-box/:quarterId/meus-liderados` | Status e botão Registrar/Editar | G |
| `/nine-box/registros/:id` | `RecordForm`, `AdjustmentDiff`, `CommentThread`, botões Mediar e Retificar conforme `can` | G |
| `/nine-box/pessoas/:personId` | Histórico de quadrantes por quarter, justificativas e comentários | G |
| `/onboarding` | Planos com progresso e atrasados | G |
| `/onboarding/novo` | Stepper Dados → Módulos (pré-marcados) → Revisão (`/plans/preview`); envia `composerStartedAt` | G |
| `/onboarding/:id` | `PlanChecklist` por seção (`Select` de status, destaque de atraso, `RingProgress`), itens avulsos, `ShareLinkManager`, Exportar .md | G |
| `/onboarding/:id/impressao` | Impressão | G |
| `/onboarding/catalogo`, `/onboarding/catalogo/:moduleId` | Módulos comuns e por time, itens, ordenação (botões subir/descer), revisões | G (edição conforme `can`) |
| `/onboarding/painel` | Em andamento, atrasados, tempo médio | G |
| `/pessoas`, `/pessoas/:id` | Diretório com links para histórico do Nine Box e pleitos | G |
| `/notificacoes` | Lista de notificações | G |
| `/admin/catalogos`, `/admin/configuracoes`, `/admin/nine-box-matriz`, `/admin/importacoes/erp`, `/admin/importacoes/candidatos`, `/admin/pessoas`, `/admin/usuarios`, `/admin/jobs`, `/admin/integracoes` | Configurações: flags, editor da matriz com validação; import ERP com upload → diff → aplicar; overrides e gestor de time | A |
| `/admin/auditoria`, `/admin/acessos`, `/admin/metricas`, `/admin/lgpd` | Auditoria, log de acesso, métricas, registro de tratamento | A\|H |

Detalhes de alguns componentes:
- **Dados restritos:** o `RestrictedPanel` só carrega depois do clique em "Mostrar dados restritos", usa `gcTime: 0` e exibe o `SensitiveDataNotice`.
- **LGPD da ficha:** `ConsentCard`, renovação de consentimento e registro de pedido.
- **`NineBoxGrid`:** grid CSS 3×3 com os rótulos do snapshot, contagem e nomes por célula. Clicar numa célula filtra a lista. Navegável por teclado, com `aria-label` por quadrante e cor sempre acompanhada de texto.
- **`RecordForm`:** dois `SegmentedControl` Baixo/Médio/Alto com prévia do quadrante e textareas com contador mínimo. O campo de motivo aparece quando o registro já foi submetido ou na mediação.
- **`ShareLinkManager`:** mostra a URL uma única vez com `CopyButton`; lista links com estado, expiração e acessos; permite revogar.

**Cards da home** (`/home`):
- Gestor: Nine Box a registrar, itens de onboarding atrasados, pleitos em rascunho ou aprovados aguardando pagamento, candidatos próprios expirando.
- Head: pleitos aguardando decisão, cobertura do Nine Box, datas-alvo vencidas, pedidos LGPD abertos, candidatos vencidos ainda em processo.
- Admin: idade do último import ERP, jobs com erro, quarter vigente ou próximo ausente.

**Página pública.** `/s/onboarding` não faz parte do SPA. São `web/share/share.html`, `share.css` e `share.js` (~150 linhas, vanilla JS):
- lê `#t=` do fragmento e faz `POST /s/api/onboarding`;
- renderiza com `textContent` e `createElement`, nunca `innerHTML`;
- mostra seções, itens, prazos e status;
- botão Imprimir (`window.print`, com CSS de impressão) e botão Baixar Markdown (Blob);
- 404 → "Link indisponível ou expirado".

**Papéis no frontend:**
- `useMe().roles` decide **apenas a navegação**: menu, `RequireRole` nas rotas e botões de criação.
- Ações sobre recursos usam só `can` e `allowedTransitions`. Não há lógica de visibilidade em TypeScript.
- Tratamento de erros:
  - 403 → "Sem permissão";
  - 404 → "Não encontrado";
  - 422 → `form.setErrors(errors)`;
  - 409 `stale_version` → `StaleVersionModal`;
  - demais → notificação com o `requestId`.
- `features.gupy` esconde a tela de integração.
- Nada vai para `localStorage` além da preferência de colunas. Não há prefetch de Nine Box nem de restritos. O logout faz `queryClient.clear()`.

---

## 9. Estratégia de testes

| Nível | O que cobre | Como roda |
|---|---|---|
| **Unitário Go** (sem banco) | Ver lista abaixo | `go test ./...`, sempre |
| **Integração Go** (MySQL real) | Ver lista abaixo | `TEST_MYSQL_DSN` definido; senão `t.Skip` |
| **Front: unitário e componente** | Ver lista abaixo | `pnpm -C web test --run` |
| **E2E** (Playwright 1.56.1, só Chromium, `workers: 1`, `retries: 0`) | Specs na §9.1 | `make e2e` |
| **Estático** | `go vet`, `go tool staticcheck`, `go tool govulncheck`, `make check-prod-deps`, `pnpm -C web lint`, `pnpm -C web typecheck` | `make lint` |

**Unitário Go** (sem banco):
- Políticas: tabelas exaustivas, persona × estado, cobrindo todos os ramos de `CanView`, `CanEdit`, `CanMediate`, `CanComment` e `CanRectify`, além de restritos, pleitos, planos e catálogo.
- FSMs: todas as combinações de/para/ator.
- Validação: `httpx.V`.
- Autenticação:
  - `returnTo`: `//x`, `/\x`, `/%09/evil.com` decodificado, `/auth/...`, `javascript:`;
  - extração de claims: array, string, caminho aninhado, ausente, overage;
  - mapeamento de grupos e gate (só-admin negado);
  - middleware CSRF;
  - `seal` (AAD trocada falha);
  - cookies (atributos em prod e em dev).
- Snapshots de auditoria: todo campo tem tag.
- Sniffing de upload: `.exe` renomeado para `.pdf`, DOCX com `vbaProject.bin`, PDF com `/JavaScript`, imagem acima de 10000², nome com `../` e CRLF.
- Normalização de LinkedIn, e-mail e tags; `search_text`.
- Datas D+n com fim de semana rolando para segunda, no fuso BRT.
- Parser CSV do ERP (`;`/`,`, BOM, `DD/MM/AAAA`, erros por linha, ciclo de hierarquia).
- Quadrante, movimentação e cobertura.
- Escape de CSV e Markdown (golden files em `testdata/golden`).
- Schedules dos jobs (cálculo de slot).
- Redação de log: o nome usado na query não aparece no buffer.

**Integração Go** (MySQL real):
- **Migrations:** banco vazio, reexecução sem efeito, checksum alterado falha, dirty bloqueia, dois runners concorrentes.
- **Módulos com SQL real:**
  - dedup e busca com filtros;
  - transições com evento e audit;
  - lote atômico;
  - `stale_version`;
  - corrida entre edição e fechamento de quarter (duas goroutines);
  - import ERP com prévia, aplicação, `preview_outdated` e trava de 20%;
  - anonimização (abaixo).
- **Matriz HTTP:** `internal/server/authz_test.go` cruza persona × rota × status esperado sobre a massa semeada. Rota nova sem entrada na tabela faz o teste falhar.
- **Equivalência SQL × política** para pleitos, planos e restritos, num dataset com troca de gestor e override.
- **Ausência de chaves** de dados restritos nos DTOs.
- **Fluxo OIDC completo** contra o `devidp` in-process: state, nonce, iss e alg errados; PKCE inválido; sem grupo; só-admin; overage; recheck com perda de grupo; `invalid_grant`; refresh concorrente de duas "unidades" sem logout espúrio; logout.
- **`access_log` fail-closed:** writer com falha → 500, sem dados na resposta.
- **Anonimização:** varre todas as colunas texto, JSON e blob do schema (via `information_schema`, exceto `onboarding_*`, que é dado de colaborador) procurando o nome, o e-mail e o telefone originais. Espera 0 ocorrências.
- **Jobs:** relógio fake; dois runners → um único `job_run` por slot; aviso único com dedupe.
- **Storage:** MySQL e S3 (`gofakes3`).
- **Desempenho** (`PERF=1`): 10 mil candidatos, busca combinada `q` + `skill≥4` + senioridade + status com p95 abaixo de 300 ms.

**Front: unitário e componente:**
- `lib/` (formatação, query string);
- `api/client` (problem → `ApiError`, 401, `X-App-Version`);
- `RequireRole`;
- `TransitionButtons` (campos por transição);
- `NineBoxGrid` (posições e rótulos do snapshot);
- `CandidateFilters` ↔ URL;
- `MoneyInput`.

**Infraestrutura de teste:**
- `dbtest.Main(m)` cria o banco `t_<pacote>_<pid>`, migra e faz `DROP` ao final.
- `dbtest.Reset(t)` faz `DELETE` de todas as tabelas, exceto `schema_migrations`, com `FOREIGN_KEY_CHECKS=0`.
- Os pacotes rodam em paralelo (`-p 4`), cada um no seu banco.
- `authtest.Session(t, db, persona)` insere usuário e sessão e devolve o cookie.
- O app de teste roda com `APP_ENV=test` e `COOKIE_SECURE=false`, e o cliente HTTP já envia `Origin`.

### 9.1 Specs E2E (`web/e2e/`)

O helper `loginAs(page, 'ana.gestora')` clica no botão da persona no devidp. Cada spec faz login no `beforeEach` (sem storageState compartilhado) e usa sufixo único nos dados que cria.

| Spec | Cenário |
|---|---|
| `auth.spec.ts` | `eva` e `fabio` caem em `/auth/denied` e `/api/v1/me` responde 401. `ana` entra; logout → `/auth/logged-out`. Grupo de `bruno` removido no devidp com `ROLE_RECHECK_INTERVAL=5s` → em até 6 s aparece "acesso revogado". |
| `security.spec.ts` | Headers CSP e nosniff presentes. Via `APIRequestContext` com o cookie: POST sem Origin → 403; com Origin estranha → 403. |
| `recruiting.spec.ts` | `ana` cria candidato com Go 4, Kafka 3 e uma tag. Alerta de duplicidade com o mesmo e-mail. Busca "Go ≥ 4" o encontra. Entrevista com scorecard. Upload de PDF ok; `.exe` renomeado é rejeitado. Funil até contratado → o plano em rascunho aparece em `/onboarding` (a partir do M8). |
| `recruiting-restricted.spec.ts` | `ana` registra a pretensão. `bruno` vê a ficha, mas não a pretensão. `bruno` reprova com motivo: `bruno` vê o próprio motivo, `ana` não vê, `helena` vê tudo. |
| `training.spec.ts` | `ana` cria e submete 2 pleitos. `helena` define o orçamento, aprova 1 e reprova 1 em lote (comentário obrigatório). `ana` marca pago com comprovante, depois em andamento e concluído com certificado. Os totais do painel conferem. `bruno` não vê o pleito de `ana` (página "Não encontrado"). O CSV é baixado. |
| `ninebox.spec.ts` (serial) | `helena` abre o quarter. `ana` registra e submete o de um liderado. `bruno` não vê o registro durante `open`. `helena` inicia a calibração. `bruno` vê e comenta. `ana` ajusta com motivo. `helena` media. `helena` fecha confirmando pendências. `ana` não consegue editar. `helena` retifica. O histórico mostra antes e depois. |
| `onboarding.spec.ts` | `alice` cria item comum. `ana` cria módulo do time dela. `ana` gera o plano e o progresso muda. O link aberto num contexto **sem cookies** mostra o plano; depois de revogado mostra "Link indisponível". |
| `admin-erp-import.spec.ts` | `alice` faz upload de `dev/sample-erp.csv` → prévia → aplicar → hierarquia aparece em `/pessoas`. |
| `lgpd.spec.ts` | `ana` registra pedido de exclusão. `helena` resolve. O candidato some da busca. |
| `a11y.spec.ts` | axe (`wcag2a`, `wcag2aa`) nas páginas principais, sem violações *serious* ou *critical* (RNF-12). |

### 9.2 Como rodar localmente

```bash
make mysql        # scripts/mysql-local.sh start
make test         # go vet + go test ./... + pnpm -C web test --run
make test-int     # TEST_MYSQL_DSN='app:app@tcp(127.0.0.1:3306)/' go test -count=1 -p 4 ./...
make lint         # vet, staticcheck, govulncheck, check-prod-deps, eslint, tsc
make seed         # go run ./cmd/devseed -dsn ... -db app_dev -reset -profile dev
make dev          # scripts/dev.sh
make e2e          # scripts/e2e.sh
make build        # pnpm -C web build (outDir ../internal/web/dist) + CGO_ENABLED=0 go build -o bin/server ./cmd/server + bin/devidp + bin/devseed
make check        # lint + test + test-int (pré-commit)
```

**`scripts/mysql-local.sh start`:**
1. Se `mysqld` não existir, roda `apt-get update && apt-get install -y mysql-server-8.0`. Se o apt estiver indisponível, falha com mensagem clara; os testes de integração continuam sendo pulados.
2. `mkdir -p /var/run/mysqld .tmp/mysql`.
3. Na primeira vez: `mysqld --initialize-insecure --user=root --datadir=$PWD/.tmp/mysql/data`.
4. Sobe em background:
   ```bash
   mysqld --user=root --datadir=$PWD/.tmp/mysql/data --socket=$PWD/.tmp/mysql/mysqld.sock \
     --pid-file=$PWD/.tmp/mysql/mysqld.pid --log-error=$PWD/.tmp/mysql/error.log \
     --port=3306 --bind-address=127.0.0.1 --mysqlx=OFF --skip-log-bin \
     --default-time-zone=+00:00 --character-set-server=utf8mb4 --collation-server=utf8mb4_0900_ai_ci \
     --sql-require-primary-key=ON --max-allowed-packet=64M --innodb-flush-log-at-trx-commit=2
   ```
5. Espera `mysqladmin ping`.
6. `CREATE USER IF NOT EXISTS 'app'@'127.0.0.1' IDENTIFIED BY 'app'; GRANT ALL ON *.* TO 'app'@'127.0.0.1';` (os testes criam bancos).

O `--user=root` é necessário porque o container roda como uid 0.

**`scripts/dev.sh`** sobe três processos:
- `bin/devidp -addr :9000 -issuer http://localhost:9000`;
- `go run ./cmd/server serve` com `PORT=8080 APP_ENV=dev APP_BASE_URL=http://localhost:5173 COOKIE_SECURE=false OIDC_ISSUER_URL=http://localhost:9000`;
- `pnpm -C web dev` (:5173), com proxy de `/api`, `/auth` e `/s` para :8080 e `changeOrigin:false`.

**`scripts/e2e.sh`:**
1. `make build`.
2. `bin/devseed -db app_e2e -reset -profile e2e`.
3. `PLAYWRIGHT_BROWSERS_PATH=/opt/pw-browsers pnpm -C web exec playwright test` (nunca `playwright install`).
4. O `webServer` sobe:
   - `bin/devidp` (url `http://localhost:9000/.well-known/openid-configuration`);
   - `bin/server serve`, **o mesmo binário de produção**, com `PORT=8080 APP_ENV=test APP_BASE_URL=http://localhost:8080 COOKIE_SECURE=false ROLE_RECHECK_INTERVAL=5s` (url `http://localhost:8080/readyz`).
5. `baseURL: http://localhost:8080`.

O devseed cria:
- 1 gerência, 4 times e cerca de 16 pessoas, com Helena → Ana, Bruno, Carla e Alice → devs;
- catálogos de skills e competências;
- quarter vigente `planned` e o próximo;
- módulos de onboarding comuns e por time.

---

## 10. Build e deploy (Tsuru, um app)

### 10.1 SPA embutido

- O Vite gera o build com `outDir: '../internal/web/dist'`, `emptyOutDir: true` e `sourcemap: false`.
- Versionamos só `internal/web/dist/robots.txt`, idêntico a `web/public/robots.txt`. O Vite o recria a cada build, então o git não acusa diferença e o `go build` funciona num clone limpo.
- `.gitignore` tem `internal/web/dist/*`, `!internal/web/dist/robots.txt` e `.tmp/`.
- Sem `index.html`, o handler responde 503 "frontend não compilado".
- `/api/*` desconhecido responde 404 em JSON.

### 10.2 Dockerfile

```dockerfile
# syntax=docker/dockerfile:1.7
FROM node:22-alpine AS web
WORKDIR /src/web
ENV CI=true PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1
RUN corepack enable && corepack prepare pnpm@10.28.0 --activate
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN mkdir -p /src/internal/web && pnpm build            # → /src/internal/web/dist

FROM golang:1.24-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOOS=linux GOTOOLCHAIN=local
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY migrations/ migrations/
COPY --from=web /src/internal/web/dist/ internal/web/dist/
# O "tsuru app deploy" não passa --build-arg: com VERSION vazia, a versão é o
# sha256 do próprio binário (internal/buildinfo), que muda a cada build.
ARG VERSION=
RUN go build -trimpath -ldflags="-s -w -X github.com/marquinhosvideo/processosplatdistr/internal/buildinfo.Version=${VERSION}" \
    -o /out/server ./cmd/server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 app
# CA interna (IdP/DBaaS), se necessário: COPY certs/*.crt /usr/local/share/ca-certificates/ && update-ca-certificates
WORKDIR /home/application/current
COPY --from=build /out/server /usr/local/bin/server
COPY Procfile tsuru.yaml ./
USER 10001
EXPOSE 8888
CMD ["/usr/local/bin/server", "serve"]
```

- Base Alpine (com shell) para que `tsuru app run` e `tsuru app shell` funcionem.
- **Versão** (`X-App-Version`, `/healthz`): o `make build` e o `make docker` gravam o sha do commit via `-X …buildinfo.Version`. O `tsuru app deploy` não passa `--build-arg` e o `.git` fica fora do contexto, então nesse caminho a versão é `bin-<12 hex do sha256 do executável>`: muda a cada build com conteúdo diferente, e o aviso "Nova versão disponível" continua funcionando.
- `WORKDIR /home/application/current` é a convenção do Tsuru para encontrar `Procfile` e `tsuru.yaml`.
- As imagens base são fixadas por digest no M10.
- `.dockerignore`: `.git`, `.tmp`, `bin`, `web/node_modules`, `web/test-results`, `web/playwright-report`, `internal/web/dist/*`, `!internal/web/dist/robots.txt`, `.env*`.

**`Procfile`:**
```
web: /usr/local/bin/server serve
```

**`tsuru.yaml`** (sem hooks, porque as migrations rodam na subida):
```yaml
healthcheck:
  path: /healthz
  method: GET
  status: 200
  scheme: http
  interval_seconds: 10
  timeout_seconds: 5
  allowed_failures: 3
```

**Deploy:**
```bash
tsuru app deploy -a processos-platdistr --dockerfile ./Dockerfile .
tsuru env set -a processos-platdistr -p OIDC_CLIENT_SECRET=… SESSION_ENC_KEY=… METRICS_TOKEN=…
tsuru service instance bind <servico-mysql> <instancia> -a processos-platdistr
```

### 10.3 Sequência de `serve`

1. `config.Load` e `Validate` (fail-fast).
2. slog.
3. `db.Open`, com retry por até 30 s.
4. Confere `VERSION() ≥ 8.0.16`.
5. Com `MIGRATE_ON_START=true` (default), roda `migrate.Up` sob `GET_LOCK`. Em caso de falha, `exit 1`: o deploy falha e o Tsuru mantém as unidades antigas.
6. Bootstrap idempotente: garante a gerência `DEFAULT_ORG_UNIT_*` e os quarters vigente e próximo.
7. Com `FILES_BACKEND=mysql`, confere `@@max_allowed_packet ≥ FILES_MAX_BYTES + 1 MB`; se falhar, erro claro.
8. Abre o HTTP com timeouts ReadHeader 10 s, Read 60 s, Write 60 s, Idle 120 s e MaxHeaderBytes 64 KB.
9. Inicia o `jobs.Runner`, se `JOBS_ENABLED`.
10. No SIGTERM: `Shutdown(20s)`, cancela os jobs (espera até 15 s) e fecha o banco.

Pool: MaxOpen 10, MaxIdle 5, ConnMaxLifetime 5 min.

**Parâmetros que `db.Open` aplica sempre:** `parseTime=true`, `loc=UTC`, `time_zone='+00:00'`, `collation=utf8mb4_0900_ai_ci`, `sql_mode='STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION,ONLY_FULL_GROUP_BY'`, `timeout=5s`, `readTimeout=30s`, `writeTimeout=30s`, `maxAllowedPacket=0`.

### 10.4 Variáveis de ambiente

S = segredo, configurado via `tsuru env set -p`. O tipo `config.Secret` aparece como `[REDACTED]` nos logs.

| Variável | Default | Observação |
|---|---|---|
| `PORT` | 8888 | Injetada pelo Tsuru |
| `APP_ENV` | production | `production`, `staging`, `dev` ou `test` |
| `APP_BASE_URL` | obrigatória | `redirect_uri = APP_BASE_URL/auth/callback`; origem para CSRF; base da URL dos links |
| `APP_TZ` | America/Sao_Paulo | Define "hoje" para retenção, prazos, quarter vigente e jobs |
| `LOG_LEVEL` | info | |
| `DATABASE_DSN` (S) | — | Formato go-sql-driver. Se vazia, lê `DATABASE_URL` (`mysql://user:pass@host:port/db`). |
| `DATABASE_URL_ENV` | `DATABASE_URL` | Nome da variável que o bind do DBaaS injeta, ex.: `DBAAS_MYSQL_ENDPOINT` |
| `MIGRATION_DATABASE_DSN` (S) | = DSN da app | Usuário com DDL |
| `MIGRATE_ON_START` | true | |
| `DB_TLS_CA_PEM` | — | Se definida, `tls=dbaas` com verificação; senão `tls=preferred` |
| `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` / `DB_CONN_MAX_LIFETIME` | 10 / 5 / 5m | |
| `OIDC_ISSUER_URL`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET` (S) | obrigatórias (o segredo pode ficar vazio) | Segredo vazio = cliente público com PKCE |
| `OIDC_SCOPES` | `openid profile email` | No Azure AD, acrescentar `offline_access` |
| `OIDC_GROUPS_CLAIM` / `OIDC_EMAIL_CLAIM` / `OIDC_NAME_CLAIM` / `OIDC_EMPLOYEE_ID_CLAIM` | groups / email / name / — | Caminho com ponto permitido |
| `ROLE_GROUPS_MANAGER`, `ROLE_GROUPS_HEAD`, `ROLE_GROUPS_ADMIN` | MANAGER obrigatória | CSV de valores exatos |
| `SESSION_ENC_KEY` (S) / `SESSION_ENC_KEY_PREVIOUS` (S) | obrigatória / — | Base64 de 32 B; a anterior só decifra (rotação) |
| `SESSION_IDLE_TIMEOUT` / `SESSION_ABSOLUTE_TIMEOUT` / `SESSION_NO_REFRESH_MAX` | 2h / 12h / 4h | |
| `ROLE_RECHECK_INTERVAL` / `ROLE_RECHECK_GRACE` | 15m / 1h | |
| `COOKIE_SECURE` | true | false só em dev/test |
| `TRUSTED_PROXY_CIDRS` | `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.0/8` | `X-Forwarded-For` só é aceito vindo dessas origens |
| `RATE_LIMIT_SHARE_PER_MIN` / `RATE_LIMIT_AUTH_PER_MIN` | 30 / 30 | Por IP, por unidade |
| `FILES_BACKEND` / `FILES_MAX_BYTES` | mysql / 10485760 | |
| `S3_ENDPOINT`, `S3_BUCKET`, `S3_REGION`, `S3_ACCESS_KEY_ID` (S), `S3_SECRET_ACCESS_KEY` (S), `S3_USE_SSL`, `S3_PATH_STYLE` | — / true / true | Só com backend `s3`. Bucket privado; o download passa pelo app. |
| `CLAMD_ADDR` / `UPLOAD_REQUIRE_SCAN` | — / false | Scanner clamd (INSTREAM) |
| `JOBS_ENABLED` | true | |
| `DEFAULT_ORG_UNIT_CODE` / `DEFAULT_ORG_UNIT_NAME` | obrigatórias | |
| `GUPY_BASE_URL`, `GUPY_TOKEN` (S) | — | Reservadas; no MVP só existe o stub |
| `METRICS_TOKEN` (S) | — | Vazia desliga `/metrics` |

**Logs** (slog JSON):
- Campos: `request_id`, `method`, `route` (o `r.Pattern`, nunca o path bruto), `status`, `duration_ms`, `user_id`.
- **Nunca** registra query string, corpo, cookies, headers de autenticação, `code`/`state` nem tokens.
- Eventos de segurança: `auth.login`, `auth.denied`, `auth.roles_lost`, `csrf.failed`, `ratelimit.hit`, `upload.rejected`.

**Métricas:** `http_requests_total{route,method,status}`, `http_request_duration_seconds{route,method}`, estatísticas do pool de conexões, `job_runs_total{job,status}`, `auth_logins_total{result}`, `share_link_access_total{result}`, `upload_rejected_total{reason}`.

**Uploads** (`files/upload.go`):
- Leitura em streaming com `r.MultipartReader()` e `io.LimitReader(max+1)`, tudo em memória (nunca `ParseMultipartForm`), mais `http.MaxBytesReader`.
- Allowlist por kind:
  - candidato: `cv` e `technical_test` aceitam pdf e docx; `portfolio` aceita pdf, png e jpeg; `other` aceita pdf, docx, png e jpeg;
  - pleito: todos os kinds aceitam pdf, png e jpeg.
- Detecção por magic bytes, que precisa bater com a extensão:
  - PDF começa com `%PDF-` e é rejeitado se casar `/(JavaScript|JS|Launch)[\s/<(\[]` (verificação best-effort);
  - DOCX é lido pelo diretório central (`archive/zip` sobre `bytes.Reader`), precisa conter `[Content_Types].xml` e `word/document.xml`, não pode conter `vbaProject.bin` e tem até 1000 entradas;
  - imagens passam por `image.DecodeConfig` com limite de 10000 × 10000.
- Nome do arquivo: basename, sem caracteres de controle, até 200 caracteres.
- Limite de `files.max_per_owner` por dono. O SHA-256 é guardado.
- Scanner: `noop` por padrão; `clamd` quando `CLAMD_ADDR` estiver definido. Infectado → 422 `malware_detected`, sem gravar.

---

## 11. Jobs em background

**Como rodam.** O `jobs.Runner` é uma goroutine em cada unidade web.
- Faz um tick por minuto, com jitter inicial de 0 a 30 s.
- Para cada job, `slot = Schedule.Slot(now em APP_TZ)` é o último instante agendado ≤ agora.
- Tenta `INSERT INTO job_runs(job_name, slot_at, origin, unit, status) VALUES (?,?,'schedule',?,'running')`:
  - chave duplicada: outra unidade já executou ou está executando, então pula;
  - sucesso: executa com timeout e grava `status`, `finished_at`, `error` e `stats`.
- Não usa `GET_LOCK`, então funciona mesmo com proxy de DBaaS que multiplexe conexões.
- Recuperação: depois de queda geral, a primeira unidade a subir executa o último slot perdido. Todos os jobs são idempotentes e processam em lotes de 100 por transação.
- Execução manual:
  - `POST /api/v1/admin/jobs/{name}/run` usa `slot_at=NOW(3)` e `origin='manual'`, e roda de forma assíncrona;
  - `tsuru app run -a processos-platdistr -- /usr/local/bin/server jobs run <nome>` roda de forma síncrona.
- Linhas `running` com mais de 2 × timeout aparecem como "travado" em `/admin/jobs`.

| Job | Agenda (APP_TZ) | O que faz |
|---|---|---|
| `session_cleanup` | De hora em hora, :05 | `DELETE FROM sessions WHERE absolute_expires_at < NOW(3) OR last_seen_at < NOW(3) - INTERVAL <idle>` |
| `candidate_retention_notice` | Diário 07:40 | Candidatos com `anonymized_at IS NULL`, `retention_warned_at IS NULL` e `retention_expires_on ≤ hoje + notice_days` (inclui os já vencidos): notifica o owner, com `dedupe_key = retention:<id>:<expires_on>`. Se o owner estiver desabilitado ou sem login há 90 dias, notifica os Heads. Grava `retention_warned_at` e audit `candidate.retention_warned`. (RF-REC-10) |
| `candidate_retention_anonymize` | Diário 03:20 | `retention_expires_on < hoje AND anonymized_at IS NULL AND retention_warned_at ≤ now − anonymize_grace_days AND status IN ('talent_pool','rejected','withdrawn','hired')` → `Anonymize`, uma tx por candidato. Candidatos em `in_process`/`approved` não são anonimizados e aparecem como "vencido — renovar" na home do owner e do Head. |
| `lgpd_request_overdue` | Diário 08:05 | Pedidos `open` com `due_on < hoje`: notifica os Heads uma vez por dia (dedupe por pedido e data) |
| `training_deadline_notice` | Diário 08:10 | Quarters com `training_deadline_on < hoje` e `training_deadline_notified_at IS NULL`: notifica os Heads com o resumo de submetidos e grava a data (RF-TRN-06) |
| `quarter_ensure` | Diário 01:05, e no bootstrap | Garante os quarters vigente e próximo (`planned`, datas padrão do trimestre civil, `created_by NULL`) |
| `share_link_cleanup` | Diário 02:10 | Apaga links expirados ou revogados há mais de 90 dias. A expiração em si é checada na leitura. |
| `access_log_purge` | Diário 04:00 | Apaga registros mais antigos que `lgpd.access_log_retention_days` |
| `erp_import_purge` | Diário 04:10 | `content=NULL` em lotes com mais de 30 dias; apaga `discarded`/`failed` com mais de 30 dias |
| `notification_purge` | Domingo 04:30 | Apaga lidas há mais de 180 dias |
| `job_runs_purge` | Domingo 04:40 | Mantém 90 dias |
| `gupy_sync` | Só manual, com `feature.gupy` | `Sync`: upsert de `job_openings` e candidatos por `(external_source, external_id)` |

**Gupy (`gupy_sync`):**
- Sem par externo, faz dedup por `email_norm`/`linkedin_handle`: vincula o ID externo ao candidato existente com evento `imported`, ou cria um novo com `legal_basis='consent'`, consentimento na data da candidatura e owner = quem disparou.
- Timeout de 10 s por chamada. Falha não afeta o app (RNF-13) e o job pode ser reexecutado.

**`recruiting.Anonymize(ctx, tx, id, actor *int64, reason)`**, usado pelo job, pela resolução de pedido de exclusão e pelo Head:

| Passo | Ação |
|---|---|
| 1. Ficha | `full_name = 'Candidato anonimizado #<id>'`; vira NULL: e-mail, `email_norm`, telefone, URLs, `linkedin_handle`, cidade, UF, `available_from`, aviso prévio, idiomas, empresa e cargo atuais, `referrer_person_id`, `referrer_name`, `source_detail`, `consent_obtained_on`, `external_source`, `external_id`; `work_models=''`; `search_text=''`; `anonymized_at=now`; `version+1` |
| 2. Exclusões | `DELETE` de `candidate_tags`, `candidate_restricted_entries` e `notifications` com `ref_type='candidate'` |
| 3. Anexos | `DELETE` de `attachments` via `candidate_attachments`, o que cascateia blobs e junção. Objetos S3 são removidos depois do commit, com 3 tentativas e log em caso de falha. |
| 4. Textos | `candidate_interviews.opinion = NULL`; `candidate_events.body = NULL`; `lgpd_requests.details = NULL` |
| 5. Registro | Evento `anonymized` e audit `candidate.anonymize` com `reason`, sem PII |

Ficam, para estatística: status, senioridade, perfil, cargo desejado, time de interesse, anos de experiência, ex-colaborador, origem, datas, skills, scores e recomendações. O `audit_log` não é tocado, porque nunca conteve PII.

---

## 12. Ordem de implementação

Cada marco é um conjunto de commits pequenos. Um marco só fecha com `make check` e `make e2e` verdes, as escritas auditadas cobertas por teste, a matriz de autorização HTTP atualizada e os textos em pt-BR.

| Marco | Commits | Pronto quando |
|---|---|---|
| **M0 Esqueleto** | `chore: módulo Go, cmd/server serve/version, config+travas, slog, httpx/problem, /healthz` · `chore(web): Vite+React+Mantine+Router+Query, AppShell vazio, robots.txt` · `build: SPA embutida via go:embed` · `build: Dockerfile, tsuru.yaml, Procfile, Makefile, .env.example` · `docs: docs/sdd.md` | `make build` gera um binário que serve o SPA; `/qualquer` → `index.html`; `/healthz` 200; `config check` rejeita config de produção insegura; `go test` e `pnpm test` verdes |
| **M1 Banco** | `feat(db): Open (DSN/URL/TLS), InTx, helpers` · `feat(db): runner de migrations + 0001_core` · `test: dbtest + scripts/mysql-local.sh` · `feat: audit.Record/Access, settings, /readyz` | Testes do runner (vazio, reexecução, checksum, dirty, concorrência); `make test-int` verde; serve falha com schema atrasado e `MIGRATE_ON_START=false` |
| **M2 Autenticação** | `feat(devidp): IdP mock + personas` · `feat(auth): login/callback/logout, sessões, seal` · `feat(auth): recheck com FOR UPDATE NOWAIT` · `feat(auth): CSRF, headers, ratelimit` · `feat(server): routes.go com RoleSet + TestEveryRouteGated` · `feat(jobs): runner + session_cleanup` · `feat(web): useMe, RequireRole, menu, logout, modal de sessão` · `test(e2e): Playwright + auth/security specs` | Todos os casos negativos de OIDC passam; só-admin negado; refresh concorrente sem logout espúrio; `check-prod-deps` verde; `auth.spec` e `security.spec` verdes (RF-AUTH-01/02, RNF-01/02) |
| **M3 Organização e admin** | `feat: 0002 + org (pessoas, times, cargos, ManagedTeams, me/reports)` · `feat(erp): CSVSource + lotes preview/apply/discard/reprocess` · `feat: overrides, gestor de time, usuários (vínculo, bloquear, revogar)` · `feat: CRUD de catálogos, settings/flags` · `feat: notificações + sino` · `feat: telas de auditoria e acesso` · `feat: devseed` | `sample-erp.csv` cria a hierarquia; segundo import idempotente; ciclo detectado; trava de 20%; `preview_outdated`; override muda "meus liderados"; E2E `admin-erp-import` (RF-AUTH-03, O1, RNF-13) |
| **M4 Recrutamento I** | `feat: 0003 + candidatos (skills, tags, idiomas, vagas)` · `feat: dedup` · `feat: busca e filtros + search_text` · `feat: funil (FSM) + timeline + notas` · `feat: owner, arquivamento, candidate_views` · `feat(web): lista, filtros, formulário, ficha` | Dedup por e-mail e LinkedIn normalizado; "Go ≥ 4" + tag + status; transição inválida → 409; evento + audit por transição; `stale_version`; desempenho com 10 mil (`PERF=1`) (RF-REC-01/02/04/05/06/07) |
| **M5 Recrutamento II** | `feat: 0004 + files (Store MySQL, sniff, Scanner noop/clamd, download)` · `feat: entrevistas + scorecards` · `feat: entradas restritas + access_log` · `feat: LGPD (consentimento, pedidos, export.json, Anonymize)` · `feat(jobs): retention_notice, retention_anonymize, lgpd_request_overdue` · `feat(web): abas Entrevistas, Anexos, Restritos, LGPD; cards da home` | `.exe` renomeado → 415; arquivo > 10 MB → 413; restritos visíveis por autor e Head; ausência de chaves no DTO; varredura de PII = 0; jobs com relógio fake; E2E recruiting, recruiting-restricted e lgpd (RF-REC-03/10, §4.1, RNF-05/06/08) |
| **M6 Treinamentos** | `feat: 0005 + quarters (CRUD, ensure)` · `feat: pleitos, beneficiários, FSM, evidências, anexos` · `feat: orçamento (gerência/time)` · `feat: painel + lote atômico` · `feat: CSV + impressão + notificações + training_deadline_notice` · `feat(web)` | FSM 100%; lote com um item inválido → nada muda; reprovação sem comentário → 409; evidência exigida; totais excluem reprovados e cancelados; equivalência SQL × política; E2E training (RF-TRN-01..07, RF-9BOX-01 parcial) |
| **M7 Nine Box** | `feat: 0006 + editor da matriz` · `feat: ciclo do quarter + participantes` · `feat: registros, submissão, ajuste, mediação` · `feat: comentários, fechamento, retificação` · `feat: matriz, histórico, cobertura, movimentação + access_log` · `feat(web): NineBoxGrid, RecordForm, calibração` | `policy_test` cobre todos os ramos; par em `open` → 404; edição depois de fechar → 409; corrida edição × fechamento; `pending_changed`; E2E ninebox (RF-9BOX-01..09) |
| **M8 Onboarding** | `feat: 0007 + catálogo com revisões e ordenação` · `feat: planos (preview, gerar, ativar, avulsos, checklist, progresso)` · `feat: links + /s (share.js) + export .md + impressão` · `feat: contratado → plano em rascunho` · `feat: painel` | D+n e fim de semana corretos; só o hash do token no banco; expirado ou revogado → 404 idêntico; rate limit 429; progresso ignora "não se aplica"; E2E onboarding e recruiting completo (RF-ONB-01..07, RF-REC-08) |
| **M9 Integrações e extras** | `feat(gupy): Client + Stub + Sync atrás da flag` · `feat(files): Store S3 (minio-go) + gofakes3` · `feat: import CSV de candidatos (R4)` · `feat: /metrics + success-metrics + /admin/lgpd` | Flag desligada → 404 e menu oculto; stub importa 3 candidatos e a segunda execução não duplica; teste S3 verde (RF-REC-09, RNF-09) |
| **M10 Endurecimento e go-live** | `test: a11y com axe` · `chore(sec): revisão da matriz HTTP, CSP final, digests` · `docs: runbook (bind do DBaaS, variáveis, rotação de chave, primeiro deploy, rollback, migrate force), docs/lgpd.md` · deploy em staging | Todos os testes verdes; `docker build` validado (dockerd local ou staging); login real em staging com os três papéis; claims e grupos conferidos; healthcheck ok; checklist LGPD com RH/Jurídico (RNF-10/11/12) |

---

## 13. Riscos técnicos e trade-offs

| # | Risco / trade-off | Mitigação |
|---|---|---|
| 1 | IdP e claims desconhecidos (Q2/P1): GUIDs e overage no Azure; grupos com `/` ou só no userinfo no Keycloak | Claim configurável por caminho, normalização da `/`, fallback para userinfo, overage negado com log, variantes no devidp. Validar com o IdP real logo depois do M2, em staging. Se houver overage, será preciso a Graph API (custo novo). |
| 2 | Recheck depende de refresh token; sem ele, a revogação no Backstage leva até 4 h | `SESSION_NO_REFRESH_MAX=4h`; revoke-sessions e bloqueio pelo Admin; recheck com lock na linha da sessão para evitar `invalid_grant` entre unidades |
| 3 | Visibilidade entre pares (registros de participantes ocultos) e Head vendo rascunhos são interpretações do PRD | Ficam numa única função pura, coberta por teste. Validar com o Head (Q3). Mudar exige só `policy.go`. |
| 4 | BLOB no MySQL: crescimento do banco e dos backups, `max_allowed_packet` | Limite de 10 MB e 20 por dono, tabela de blob separada, checagem na subida, `attachments.storage` por linha permitindo migração gradual para S3 (Q5) |
| 5 | Restrições do DBaaS: CHECK ≥ 8.0.16, colunas geradas, `FOR UPDATE NOWAIT` (8.0.1+), usuário sem DDL, DDL não transacional | Verificação de versão na subida; `MIGRATION_DATABASE_DSN`; migrations pequenas e idempotentes; `dirty` + `migrate force`; local com `sql_require_primary_key=ON` para imitar o DBaaS |
| 6 | Migration longa atrasa o healthcheck | Migrations pequenas; `allowed_failures`; `server migrate` manual para as pesadas |
| 7 | Jobs dentro do processo web dependem de ao menos uma unidade viva | Claim por `UNIQUE(job_name, slot_at)`, sem depender de lock de sessão; catch-up do último slot; `/admin/jobs` e execução manual |
| 8 | Sem e-mail/Slack: avisos só para quem abre o app (R4) | Sino de notificações e home de pendências. As mesmas consultas viram envio no pós-MVP. |
| 9 | Auditoria sem valores de PII ou de conteúdo do Nine Box | O valor anterior exigido pelo RNF-07 fica nas tabelas de evento de domínio, sob a regra de visibilidade. Isso evita reescrever o `audit_log` na anonimização. |
| 10 | SQL escrito à mão pode divergir entre lista e detalhe | Nine Box filtrado em Go pelo `CanView`; testes de equivalência para pleitos, planos e restritos; integração em todo método |
| 11 | CSV do ERP parcial (Q1) desativaria pessoas | Lote com prévia, trava de 20%, `preview_outdated`, reprocessamento e vínculo manual de usuários |
| 12 | Token do link no fragmento: fica no histórico do navegador do onboardee; a página exige JS | Dados mínimos, expiração, revogação, `access_count` visível ao gestor; JS vanilla com CSP `script-src 'self'` e sem `innerHTML`. O token nunca chega a logs de router nem a `Referer`. |
| 13 | Versões de dependências × Go 1.24; Playwright × Chromium pré-instalado | `GOTOOLCHAIN=local`, versões fixadas, `@playwright/test@1.56.1` exato, `PLAYWRIGHT_BROWSERS_PATH=/opt/pw-browsers` |
| 14 | Tsuru: leitura de `tsuru.yaml`/`Procfile` em deploy por Dockerfile, nome da variável do bind, CA interna | `WORKDIR /home/application/current`, `DATABASE_URL_ENV`, linha de CA no Dockerfile. Validar no primeiro deploy em staging. |
| 15 | Busca por LIKE acima de 10 mil registros | `search_text` único atende ao RNF-11 (medido). A partir de 100 mil, FULLTEXT ngram sem mudar a API. |
| 16 | Rate limit em memória fica por unidade | Aceitável: o token tem 256 bits. Limitador no router fica para depois. |
| 17 | Perda da `SESSION_ENC_KEY` | Só derruba sessões, pois não há dado de negócio cifrado. Rotação com `_PREVIOUS`. |
| 18 | Regras assumidas que precisam de aval do PO | Reabertura do funil desligada por padrão; evidências de pago e concluído; allowlist de anexos; justificativa mínima de 30 caracteres; ManagedTeams; aprovação não bloqueada por orçamento. Todas isoladas em funções puras ou em settings. |
| 19 | Expansão para várias gerências | `org_unit_id` está em todas as raízes de agregado e predicados, e o `Principal.OrgUnitID` já existe. Catálogos são globais de propósito. Será preciso um seletor de gerência quando o usuário pertencer a mais de uma (pós-MVP). |

---

## Decisões e justificativas

- **Monólito modular em duas camadas, com stdlib onde der** (ServeMux, `database/sql`, slog, runner de migration próprio). É o menor custo de implementação e manutenção para cerca de 50 usuários e um app Tsuru, com 6 dependências diretas no backend.
- **Gate exige o papel gestor; Head ⇒ Gestor; Admin é aditivo e sozinho é negado.** Cumpre o RF-AUTH-02 e a decisão do usuário.
- **Sessão opaca no MySQL, com recheck de papéis a cada 15 min** via refresh token e `FOR UPDATE NOWAIT`. O grupo do Backstage passa a valer durante a sessão, sem corrida entre unidades nem rotação do ID da sessão.
- **CSRF por `Sec-Fetch-Site`/`Origin` + `SameSite=Lax` + JSON obrigatório, sem token sincronizador.** O `Lax` é necessário para a navegação que volta do IdP. Não há CORS.
- **`/healthz` sem banco para o Tsuru e `/readyz` com banco.** Uma oscilação do DBaaS não derruba todas as unidades.
- **Migrations na subida, sob `GET_LOCK`, sem hook.** O comportamento é o mesmo com ou sem leitura de hooks em deploy por Dockerfile, e uma falha impede o deploy.
- **Jobs no processo, com claim por `UNIQUE(job_name, slot_at)`.** Não precisa de worker nem de cron externo, e é seguro com N unidades e com proxy de DBaaS.
- **IDs `BIGINT`, com autorização em toda rota e 404 para objeto invisível.** Simples de depurar. A não-enumerabilidade só importa no link público, que usa um token de 256 bits.
- **Visibilidade do Nine Box numa única função pura**, usada também para filtrar listas. Fonte única de verdade. Pares são os participantes, configuráveis via `ninebox_participants` e `peer_policy`, e incluem gestores que nunca logaram.
- **Head media a calibração** ajustando registros submetidos, com motivo. Cobre o "Head media" do RF-9BOX-03.
- **Dados restritos por entrada, com autor.** Cumpre "quem registrou" mesmo quando outro gestor escreve o motivo de reprovação.
- **Funil fiel ao PRD:** Reprovado e Desistiu são terminais. Reabrir é só do Head, atrás de setting desligada.
- **Versionamento real do catálogo** com snapshot por revisão, mantendo `source_module_version` nos itens do plano. Não há fluxo de rascunho e publicação, que seria complexidade sem pedido.
- **Orçamento por gerência ou por time, via setting com as duas opções implementadas** (`training_budgets.team_id`). Configurabilidade efetiva da Q6.
- **Anexos com tabelas de junção por dono** (candidato e pleito). FKs íntegras, sem CHECK polimórfico, e cada módulo cria sua junção na própria migration.
- **Auditoria append-only com política de campo plain/redacted.** Não guarda PII e a anonimização não precisa tocar nela.
- **Link de onboarding com token no fragmento e página mínima em JS vanilla.** O token não aparece em logs de router nem em `Referer`.
- **Recusados:** cifra de campo, cadeia de hash, outbox, OpenAPI com geração de código, ULID/UUID, chi, sqlx/squirrel, validator, zod e MSW. São custo sem requisito M correspondente.

**Pareceres dos juízes e como foram resolvidos neste SDD:**

| Falha apontada no "pragmático" | Correção |
|---|---|
| Admin sozinho entrava | Gate (§5.1) e persona `fabio.soadmin` |
| Papéis congelados por 12 h | Recheck (§7.5) |
| `/healthcheck` com banco | `/healthz` e `/readyz` |
| Open redirect em `returnTo` | Validação com `url.Parse` e rejeição de controle (§7.2) |
| FK ao excluir rascunho de pleito | Junções e ordem de exclusão (§3.7) |
| Janela de execução dupla de jobs | Claim por slot (§11) |
| Aviso de retenção passivo | Notificação, `retention_warned_at` e carência |
| Versionamento era só contador | `onboarding_module_revisions` |
| Sem vagas | `job_openings` e `candidate_openings` |
| Sem scanner de malware | `files.Scanner` e allowlist por kind |
| `/metrics` público | `METRICS_TOKEN` |
| Pares não configuráveis | `ninebox_participants` e `peer_policy` |
| Orçamento não configurável | `training.budget_scope` |
| Reabertura do funil sem aval | Setting só do Head, desligada |
| Restritos ligados ao owner | Entradas por autor |
| Gestor editava trilha e criava plano de qualquer time | ManagedTeams |
| `id_token` em claro | `seal` |
| Sem TLS no DBaaS | `DB_TLS_CA_PEM` |
| Token do link no path | Fragmento |
| Sem testes de componente no front | Vitest + Testing Library |
| Cookie `Secure` em teste http | `COOKIE_SECURE=false` só em dev/test |
| `mysqld` como root | `--user=root` no script |
| Sem `lgpd_requests` | Tabela e fluxo de pedidos |
| Sem métricas de sucesso | `candidate_views`, `user_activity_days`, `/admin/success-metrics` |
| Sem migração das planilhas (R4) | Import CSV de candidatos |
| Sem registro de base legal por tipo de dado | `lgpd.processing_records` |
| Sem PDF (RF-TRN-07) | Impressão do navegador |
| Sem acessibilidade testada | E2E com axe |
