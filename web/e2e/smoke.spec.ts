import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';

// Esqueleto (M0): o binário de produção serve o SPA embutido, /healthz e o
// 404 JSON da API, com CSP e cache da §7.8, sem erros no console.

/** Junta erros do console e exceções da página (ex.: violação de CSP). */
function collectErrors(page: Page): string[] {
  const errors: string[] = [];
  page.on('console', (msg) => {
    if (msg.type() === 'error') errors.push(msg.text());
  });
  page.on('pageerror', (err) => errors.push(err.message));
  return errors;
}

test('/healthz responde 200 com a versão do binário', async ({ request }) => {
  const res = await request.get('/healthz');
  expect(res.status()).toBe(200);
  expect(res.headers()['cache-control']).toBe('no-store');
  const body = (await res.json()) as { status: string; version: string };
  expect(body.status).toBe('ok');
  expect(body.version).not.toBe('');
  expect(res.headers()['x-app-version']).toBe(body.version);
});

test('a página inicial carrega a casca em pt-BR sem erros', async ({ page }) => {
  const errors = collectErrors(page);
  const res = await page.goto('/');

  expect(res?.status()).toBe(200);
  const headers = res?.headers() ?? {};
  expect(headers['content-security-policy']).toContain("script-src 'self'");
  expect(headers['x-content-type-options']).toBe('nosniff');
  expect(headers['cache-control']).toBe('no-cache');

  await expect(page.getByRole('heading', { level: 1, name: 'Início' })).toBeVisible();
  await expect(page.getByRole('banner').getByRole('link', { name: 'Ciclo do Colaborador' })).toBeVisible();
  await expect(page.getByRole('navigation', { name: 'Navegação principal' }).getByRole('link', { name: 'Início' })).toHaveAttribute(
    'aria-current',
    'page',
  );
  await expect(page.locator('html')).toHaveAttribute('lang', 'pt-BR');
  await expect(page).toHaveTitle('Início · Ciclo do Colaborador');
  expect(errors).toEqual([]);
});

test('qualquer rota de página devolve o index.html e o SPA mostra "Página não encontrada"', async ({ page }) => {
  const errors = collectErrors(page);
  const res = await page.goto('/qualquer/rota/profunda?x=1');

  expect(res?.status()).toBe(200);
  expect(res?.headers()['content-type']).toBe('text/html; charset=utf-8');
  await expect(page.getByRole('heading', { level: 1, name: 'Página não encontrada' })).toBeVisible();

  await page.getByRole('link', { name: 'Voltar ao início' }).click();
  await expect(page).toHaveURL('/');
  await expect(page.getByRole('heading', { level: 1, name: 'Início' })).toBeVisible();
  expect(errors).toEqual([]);
});

test('assets do Vite saem com cache imutável e o robots.txt bloqueia indexação', async ({ page, request }) => {
  await page.goto('/');
  const script = await page.locator('script[type="module"][src^="/assets/"]').first().getAttribute('src');
  expect(script).toMatch(/^\/assets\/index-[\w-]+\.js$/);

  const asset = await request.get(script ?? '');
  expect(asset.status()).toBe(200);
  expect(asset.headers()['cache-control']).toBe('public, max-age=31536000, immutable');
  expect(asset.headers()['content-type']).toBe('text/javascript; charset=utf-8');

  const missing = await request.get('/assets/nao-existe-123.js');
  expect(missing.status()).toBe(404);

  const robots = await request.get('/robots.txt');
  expect(await robots.text()).toContain('Disallow: /');
});

test('rota desconhecida da API responde 404 em problem+json', async ({ request }) => {
  const res = await request.get('/api/v1/nao-existe');
  expect(res.status()).toBe(404);
  expect(res.headers()['content-type']).toBe('application/problem+json');
  expect(res.headers()['cache-control']).toBe('no-store');
  const problem = (await res.json()) as Record<string, unknown>;
  expect(problem).toMatchObject({ status: 404, code: 'not_found', title: 'Não encontrado', type: 'urn:pp:error:not_found' });
  expect(problem.requestId).toBe(res.headers()['x-request-id']);
});

test('no celular a navegação abre pelo botão de menu', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/qualquer');
  const nav = page.getByRole('navigation', { name: 'Navegação principal' });
  await expect(nav).not.toBeInViewport();

  await page.getByRole('button', { name: 'Abrir menu de navegação' }).click();
  await expect(nav).toBeInViewport();
  await nav.getByRole('link', { name: 'Início' }).click();

  await expect(page.getByRole('heading', { level: 1, name: 'Início' })).toBeVisible();
  await expect(nav).not.toBeInViewport();
});

test('o link "Pular para o conteúdo" aparece com o teclado', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { level: 1, name: 'Início' })).toBeVisible();
  const skip = page.getByRole('link', { name: 'Pular para o conteúdo' });
  await expect(skip).not.toBeInViewport();

  await page.keyboard.press('Tab');
  await expect(skip).toBeFocused();
  await expect(skip).toBeInViewport();
});

for (const path of ['/', '/qualquer']) {
  test(`a11y: ${path} sem violações sérias ou críticas (WCAG 2 A/AA)`, async ({ page }) => {
    await page.goto(path);
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible();

    const { violations } = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']).analyze();
    const serious = violations
      .filter((v) => v.impact === 'serious' || v.impact === 'critical')
      .map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(' ')).join(', ')}`);
    expect(serious).toEqual([]);
  });
}
