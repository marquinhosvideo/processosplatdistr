import { defineConfig, devices } from '@playwright/test';

// E2E contra o binário de produção (SDD §9.2). Rode por scripts/e2e.sh (make
// e2e), que compila bin/server antes. O Chromium vem de /opt/pw-browsers
// (PLAYWRIGHT_BROWSERS_PATH); nunca rode "playwright install".

const port = Number(process.env.E2E_PORT ?? 8080);
const baseURL = `http://localhost:${port}`;

// Configuração de teste do server: cookies sem Secure só valem com APP_ENV=test.
const serverEnv: Record<string, string> = {
  APP_ENV: 'test',
  PORT: String(port),
  APP_BASE_URL: baseURL,
  LOG_LEVEL: 'warn',
  COOKIE_SECURE: 'false',
  OIDC_ISSUER_URL: 'http://localhost:9000',
  OIDC_CLIENT_ID: 'processos-dev',
  OIDC_CLIENT_SECRET: 'dev-secret',
  ROLE_GROUPS_MANAGER: 'gestores',
  ROLE_GROUPS_HEAD: 'head',
  ROLE_GROUPS_ADMIN: 'admins',
  ROLE_RECHECK_INTERVAL: '5s',
  // Chave fixa e pública: só serve para o ambiente de teste.
  SESSION_ENC_KEY: Buffer.from('e2e-only-session-key-32-bytes!!!').toString('base64'),
  DEFAULT_ORG_UNIT_CODE: 'PLATDIST',
  DEFAULT_ORG_UNIT_NAME: 'Plataformas de Distribuição',
  DATABASE_DSN: process.env.E2E_DATABASE_DSN ?? 'app:app@tcp(127.0.0.1:3306)/app_e2e',
};

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  forbidOnly: Boolean(process.env.CI),
  reporter: [['list'], ['html', { open: 'never' }]],
  timeout: 30_000,
  expect: { timeout: 6_000 },
  use: {
    baseURL,
    locale: 'pt-BR',
    timezoneId: 'America/Sao_Paulo',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: [
    {
      command: '../bin/server serve',
      // /healthz não depende do banco; passa a ser /readyz quando o server
      // abrir o MySQL na subida.
      url: `${baseURL}/healthz`,
      env: serverEnv,
      reuseExistingServer: false,
      timeout: 60_000,
      stdout: 'ignore',
      stderr: 'pipe',
    },
  ],
});
