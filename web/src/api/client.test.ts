import { describe, expect, it, vi } from 'vitest';

import { api, ApiError, buildURL, isApiError, NETWORK_ERROR, onUnauthorized, onVersionChange, request, upload } from './client';
import { shouldRetry } from './queryClient';

type FetchArgs = [input: string, init: RequestInit];

function respond(status: number, body?: unknown, headers: Record<string, string> = {}) {
  const text = body === undefined ? null : typeof body === 'string' ? body : JSON.stringify(body);
  return new Response(text, { status, headers });
}

/**
 * Cada chamada consome a próxima resposta; um erro é rejeitado. O teste é por
 * Response porque o DOMException do jsdom não herda de Error.
 */
function mockFetch(...responses: (Response | Error)[]) {
  const fn = vi.fn<(...args: FetchArgs) => Promise<Response>>(() => {
    const next = responses.shift();
    if (next === undefined) throw new Error('fetch inesperado');
    return next instanceof Response ? Promise.resolve(next) : Promise.reject(next);
  });
  vi.stubGlobal('fetch', fn);
  return fn;
}

function call(fn: ReturnType<typeof mockFetch>, i = 0): FetchArgs {
  const args = fn.mock.calls[i];
  if (!args) throw new Error(`fetch não foi chamado ${i + 1} vez(es)`);
  return args;
}

// Corpo no formato de httpx.writeProblem (internal/httpx/problem.go).
const validationProblem = {
  type: 'urn:pp:error:validation_failed',
  title: 'Dados inválidos',
  status: 422,
  code: 'validation_failed',
  detail: 'Corrija os campos destacados.',
  errors: { email: 'inválido' },
  requestId: '0123456789abcdef',
};

describe('request', () => {
  it('faz GET same-origin pedindo JSON e devolve o corpo', async () => {
    const fetchFn = mockFetch(respond(200, { status: 'ok', version: 'abc' }, { 'Content-Type': 'application/json' }));

    await expect(api.get('/healthz')).resolves.toEqual({ status: 'ok', version: 'abc' });

    const [url, init] = call(fetchFn);
    expect(url).toBe('/healthz');
    expect(init.method).toBe('GET');
    expect(init.credentials).toBe('same-origin');
    expect(init.body).toBeUndefined();
    const headers = new Headers(init.headers);
    expect(headers.get('Accept')).toContain('application/json');
    expect(headers.has('Content-Type')).toBe(false);
  });

  it('envia o corpo como JSON com Content-Type nas escritas', async () => {
    const fetchFn = mockFetch(respond(201, { id: 7 }, { 'Content-Type': 'application/json' }));

    await expect(api.post('/api/v1/coisas', { nome: 'Ana' })).resolves.toEqual({ id: 7 });

    const [, init] = call(fetchFn);
    expect(init.method).toBe('POST');
    expect(init.body).toBe('{"nome":"Ana"}');
    expect(new Headers(init.headers).get('Content-Type')).toBe('application/json');
  });

  it.each([
    ['DELETE', () => api.delete('/api/v1/coisas/1')],
    ['POST sem corpo', () => api.post('/api/v1/notifications/read-all')],
  ])('%s também manda Content-Type JSON, que o servidor exige em toda escrita', async (_name, send) => {
    const fetchFn = mockFetch(respond(204));

    await send();

    const [, init] = call(fetchFn);
    expect(init.body).toBeUndefined();
    expect(new Headers(init.headers).get('Content-Type')).toBe('application/json');
  });

  it('devolve undefined para 204 sem corpo', async () => {
    mockFetch(respond(204));
    await expect(api.delete('/api/v1/coisas/1')).resolves.toBeUndefined();
  });

  it('recusa sucesso que não é JSON', async () => {
    mockFetch(respond(200, '<html></html>', { 'Content-Type': 'text/html' }));
    await expect(api.get('/api/v1/coisas')).rejects.toMatchObject({ code: 'internal_error', status: 200 });
  });

  it('upload manda o FormData sem Content-Type, para o navegador pôr o boundary', async () => {
    const fetchFn = mockFetch(respond(201, { id: 1 }, { 'Content-Type': 'application/json' }));
    const form = new FormData();
    form.append('kind', 'cv');

    await upload('/api/v1/candidates/1/attachments', form);

    const [, init] = call(fetchFn);
    expect(init.method).toBe('POST');
    expect(init.body).toBe(form);
    expect(new Headers(init.headers).has('Content-Type')).toBe(false);
  });
});

describe('erros', () => {
  it('converte problem+json em ApiError com erros por campo e requestId', async () => {
    mockFetch(respond(422, validationProblem, { 'Content-Type': 'application/problem+json' }));

    const err: unknown = await api.put('/api/v1/coisas/1', { version: 1 }).catch((e: unknown) => e);

    expect(err).toBeInstanceOf(ApiError);
    expect(isApiError(err, 'validation_failed')).toBe(true);
    expect(err).toMatchObject({
      status: 422,
      code: 'validation_failed',
      title: 'Dados inválidos',
      detail: 'Corrija os campos destacados.',
      message: 'Corrija os campos destacados.',
      fieldErrors: { email: 'inválido' },
      requestId: '0123456789abcdef',
    });
  });

  it('preserva os membros de extensão do problem', async () => {
    const matches = [{ id: 3, fullName: 'Ana' }];
    mockFetch(
      respond(
        409,
        { type: 'urn:pp:error:possible_duplicate', title: 'Possível duplicidade', status: 409, code: 'possible_duplicate', matches },
        { 'Content-Type': 'application/problem+json' },
      ),
    );

    const err = (await api.post('/api/v1/candidates', {}).catch((e: unknown) => e)) as ApiError;

    expect(err.code).toBe('possible_duplicate');
    expect(err.problem.matches).toEqual(matches);
    expect(err.fieldErrors).toEqual({});
  });

  it('usa o X-Request-Id do header quando o 500 não traz corpo legível', async () => {
    mockFetch(respond(500, 'oops', { 'Content-Type': 'text/plain', 'X-Request-Id': 'feedfacecafebeef' }));

    await expect(api.get('/api/v1/coisas')).rejects.toMatchObject({
      status: 500,
      code: 'internal_error',
      requestId: 'feedfacecafebeef',
    });
  });

  it('gera um erro coerente para status sem problem+json (gateway)', async () => {
    mockFetch(respond(503, '<h1>Service Unavailable</h1>', { 'Content-Type': 'text/html' }));

    await expect(api.get('/api/v1/coisas')).rejects.toMatchObject({
      status: 503,
      code: 'unavailable',
      title: 'Serviço indisponível',
    });
  });

  it('falha de rede vira ApiError status 0 com mensagem em pt-BR', async () => {
    mockFetch(new TypeError('Failed to fetch'));

    await expect(api.get('/api/v1/coisas')).rejects.toMatchObject({
      status: 0,
      code: NETWORK_ERROR,
      message: expect.stringContaining('Não foi possível falar com o servidor') as unknown,
    });
  });

  it('cancelamento continua sendo AbortError', async () => {
    const abort = new DOMException('The operation was aborted.', 'AbortError');
    mockFetch(abort);

    await expect(api.get('/api/v1/coisas')).rejects.toBe(abort);
  });

  it('avisa os listeners de 401 com o código da sessão', async () => {
    const seen: string[] = [];
    const off = onUnauthorized((e) => seen.push(e.code));
    const expired = { type: 'urn:pp:error:session_expired', title: 'Sessão expirada', status: 401, code: 'session_expired' };
    mockFetch(
      respond(401, expired, { 'Content-Type': 'application/problem+json' }),
      respond(403, { ...expired, status: 403, code: 'forbidden', title: 'Sem permissão' }, { 'Content-Type': 'application/problem+json' }),
      respond(401, { ...expired, code: 'access_revoked' }, { 'Content-Type': 'application/problem+json' }),
    );

    await expect(api.get('/api/v1/me')).rejects.toMatchObject({ status: 401 });
    await expect(api.get('/api/v1/me')).rejects.toMatchObject({ status: 403 });
    off();
    await expect(api.get('/api/v1/me')).rejects.toMatchObject({ code: 'access_revoked' });

    expect(seen).toEqual(['session_expired']);
  });
});

describe('X-App-Version', () => {
  it('avisa uma única vez quando a versão do servidor muda', async () => {
    const versions: string[] = [];
    onVersionChange((v) => versions.push(v));
    const json = (v: string) => respond(200, {}, { 'Content-Type': 'application/json', 'X-App-Version': v });
    mockFetch(json('v1'), json('v1'), json('v2'), json('v3'));

    for (let i = 0; i < 4; i++) await request('/api/v1/x');

    expect(versions).toEqual(['v2']);
  });

  it('também observa a versão nas respostas de erro', async () => {
    const versions: string[] = [];
    onVersionChange((v) => versions.push(v));
    mockFetch(
      respond(200, {}, { 'Content-Type': 'application/json', 'X-App-Version': 'v1' }),
      respond(404, { title: 'Não encontrado', code: 'not_found', status: 404 }, { 'Content-Type': 'application/problem+json', 'X-App-Version': 'v2' }),
    );

    await request('/api/v1/x');
    await request('/api/v1/y').catch(() => undefined);

    expect(versions).toEqual(['v2']);
  });
});

describe('buildURL', () => {
  it('omite vazios e repete parâmetros multivalorados', () => {
    expect(
      buildURL('/api/v1/candidates', { q: 'go dev', status: ['in_process', 'approved'], page: 2, mine: true, tag: '', team: null, x: undefined }),
    ).toBe('/api/v1/candidates?q=go+dev&status=in_process&status=approved&page=2&mine=true');
  });

  it('mantém o caminho sem query e acrescenta a uma query existente', () => {
    expect(buildURL('/api/v1/x')).toBe('/api/v1/x');
    expect(buildURL('/api/v1/x', { a: undefined })).toBe('/api/v1/x');
    expect(buildURL('/api/v1/x?a=1', { b: 2 })).toBe('/api/v1/x?a=1&b=2');
  });
});

describe('shouldRetry', () => {
  const err = (status: number) => new ApiError({ type: '', title: '', status, code: 'x' });

  it('não repete erros 4xx', () => {
    expect(shouldRetry(0, err(404))).toBe(false);
    expect(shouldRetry(0, err(401))).toBe(false);
  });

  it('repete rede e 5xx até duas vezes', () => {
    expect(shouldRetry(0, err(0))).toBe(true);
    expect(shouldRetry(1, err(503))).toBe(true);
    expect(shouldRetry(2, err(503))).toBe(false);
    expect(shouldRetry(0, new Error('qualquer'))).toBe(true);
  });
});
