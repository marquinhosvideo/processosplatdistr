import type { Problem } from './types';

// Cliente HTTP do SPA (SDD §8.1): fetch com cookie de sessão same-origin, JSON
// nos dois sentidos e erro problem+json convertido em ApiError.

const APP_VERSION_HEADER = 'X-App-Version';

/** Código local para falha de rede (o servidor nunca o devolve). */
export const NETWORK_ERROR = 'network_error';

// Título e código usados quando a resposta de erro não é problem+json (proxy,
// gateway, servidor fora do ar).
const fallbackByStatus: Record<number, { code: string; title: string }> = {
  400: { code: 'bad_request', title: 'Requisição inválida' },
  401: { code: 'unauthenticated', title: 'Não autenticado' },
  403: { code: 'forbidden', title: 'Sem permissão' },
  404: { code: 'not_found', title: 'Não encontrado' },
  413: { code: 'payload_too_large', title: 'Conteúdo grande demais' },
  429: { code: 'rate_limited', title: 'Muitas requisições' },
  502: { code: 'unavailable', title: 'Serviço indisponível' },
  503: { code: 'unavailable', title: 'Serviço indisponível' },
  504: { code: 'unavailable', title: 'Serviço indisponível' },
};

export class ApiError extends Error {
  override name = 'ApiError';
  /** Status HTTP; 0 quando a requisição nem chegou ao servidor. */
  readonly status: number;
  readonly code: string;
  readonly title: string;
  readonly detail: string | undefined;
  /** Erros por campo do 422, prontos para `form.setErrors`. */
  readonly fieldErrors: Record<string, string>;
  readonly requestId: string | undefined;
  /** Corpo completo, com os membros de extensão (`matches`, `id`…). */
  readonly problem: Problem;

  constructor(problem: Problem) {
    super(problem.detail ?? problem.title);
    this.status = problem.status;
    this.code = problem.code;
    this.title = problem.title;
    this.detail = problem.detail;
    this.fieldErrors = problem.errors ?? {};
    this.requestId = problem.requestId;
    this.problem = problem;
  }
}

export function isApiError(err: unknown, code?: string): err is ApiError {
  return err instanceof ApiError && (code === undefined || err.code === code);
}

// --- eventos globais: 401 e nova versão ---

type Listener<T> = (value: T) => void;

const unauthorizedListeners = new Set<Listener<ApiError>>();
const versionListeners = new Set<Listener<string>>();
let knownVersion: string | undefined;
let versionAnnounced = false;

/**
 * Avisa a cada 401 da API (`unauthenticated`, `session_expired`,
 * `access_revoked`). O SPA não redireciona sozinho: mostra o modal de sessão.
 */
export function onUnauthorized(listener: Listener<ApiError>): () => void {
  unauthorizedListeners.add(listener);
  return () => {
    unauthorizedListeners.delete(listener);
  };
}

/**
 * Avisa, uma única vez, quando o servidor passa a responder com outra
 * X-App-Version. A referência é a primeira versão vista nesta aba: SPA e API
 * saem no mesmo binário, então ela é a versão do SPA carregado.
 */
export function onVersionChange(listener: Listener<string>): () => void {
  versionListeners.add(listener);
  return () => {
    versionListeners.delete(listener);
  };
}

function observeVersion(res: Response): void {
  const version = res.headers.get(APP_VERSION_HEADER);
  if (!version) return;
  if (knownVersion === undefined) {
    knownVersion = version;
    return;
  }
  if (version !== knownVersion && !versionAnnounced) {
    versionAnnounced = true;
    versionListeners.forEach((fn) => {
      fn(version);
    });
  }
}

/** Zera versão vista e listeners; só para testes. */
export function resetClientState(): void {
  unauthorizedListeners.clear();
  versionListeners.clear();
  knownVersion = undefined;
  versionAnnounced = false;
}

// --- requisições ---

export type QueryValue = string | number | boolean | null | undefined;
export type Query = Record<string, QueryValue | readonly QueryValue[]>;

export interface RequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
  /** Serializado como JSON; FormData segue como multipart (ver upload). */
  body?: unknown;
  /** Valores vazios são omitidos; arrays repetem o parâmetro (`status=a&status=b`). */
  query?: Query;
  signal?: AbortSignal;
}

export function buildURL(path: string, query?: Query): string {
  if (!query) return path;
  const params = new URLSearchParams();
  for (const [key, raw] of Object.entries(query)) {
    const values: readonly QueryValue[] = Array.isArray(raw) ? raw : [raw as QueryValue];
    for (const v of values) {
      if (v === undefined || v === null || v === '') continue;
      params.append(key, String(v));
    }
  }
  const qs = params.toString();
  return qs ? `${path}${path.includes('?') ? '&' : '?'}${qs}` : path;
}

/** Faz a requisição e devolve o JSON da resposta (undefined para corpo vazio). */
export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = 'GET', body, query, signal } = options;
  const headers = new Headers({ Accept: 'application/json, application/problem+json' });
  let payload: BodyInit | undefined;
  if (body instanceof FormData) {
    payload = body; // o navegador define o boundary do multipart
  } else if (body !== undefined) {
    headers.set('Content-Type', 'application/json');
    payload = JSON.stringify(body);
  }

  let res: Response;
  try {
    res = await fetch(buildURL(path, query), {
      method,
      headers,
      body: payload,
      credentials: 'same-origin',
      signal,
    });
  } catch (err) {
    // Cancelamento (troca de página, React Query) não é erro de rede.
    if (isAbort(err)) throw err;
    throw new ApiError({
      type: `urn:pp:error:${NETWORK_ERROR}`,
      title: 'Sem conexão',
      status: 0,
      code: NETWORK_ERROR,
      detail: 'Não foi possível falar com o servidor. Verifique a conexão e tente de novo.',
    });
  }

  observeVersion(res);
  if (!res.ok) {
    const error = new ApiError(await readProblem(res));
    if (error.status === 401) {
      unauthorizedListeners.forEach((fn) => {
        fn(error);
      });
    }
    throw error;
  }
  return readBody<T>(res);
}

/** Envia arquivos em multipart/form-data (rotas de upload). */
export function upload<T>(path: string, form: FormData, options: Omit<RequestOptions, 'body'> = {}): Promise<T> {
  return request<T>(path, { method: 'POST', ...options, body: form });
}

export const api = {
  get: <T>(path: string, options?: Omit<RequestOptions, 'method' | 'body'>) => request<T>(path, options),
  post: <T>(path: string, body?: unknown, options?: Omit<RequestOptions, 'method' | 'body'>) =>
    request<T>(path, { ...options, method: 'POST', body }),
  put: <T>(path: string, body: unknown, options?: Omit<RequestOptions, 'method' | 'body'>) =>
    request<T>(path, { ...options, method: 'PUT', body }),
  patch: <T>(path: string, body: unknown, options?: Omit<RequestOptions, 'method' | 'body'>) =>
    request<T>(path, { ...options, method: 'PATCH', body }),
  delete: <T>(path: string, options?: Omit<RequestOptions, 'method' | 'body'>) =>
    request<T>(path, { ...options, method: 'DELETE' }),
};

function isAbort(err: unknown): boolean {
  return typeof err === 'object' && err !== null && 'name' in err && err.name === 'AbortError';
}

function isJSON(res: Response): boolean {
  const type = res.headers.get('Content-Type') ?? '';
  return /^application\/(problem\+)?json\b/i.test(type);
}

async function readBody<T>(res: Response): Promise<T> {
  const text = await res.text();
  if (text === '') return undefined as T;
  if (!isJSON(res)) {
    throw new ApiError({
      type: 'urn:pp:error:internal_error',
      title: 'Resposta inesperada',
      status: res.status,
      code: 'internal_error',
      detail: 'O servidor respondeu num formato inesperado.',
      requestId: res.headers.get('X-Request-Id') ?? undefined,
    });
  }
  return JSON.parse(text) as T;
}

async function readProblem(res: Response): Promise<Problem> {
  const requestId = res.headers.get('X-Request-Id') ?? undefined;
  if (isJSON(res)) {
    try {
      const body: unknown = await res.json();
      if (isProblem(body)) return { requestId, ...body, status: res.status };
    } catch {
      // corpo ilegível: cai no problem genérico abaixo
    }
  }
  const fallback = fallbackByStatus[res.status] ?? { code: 'internal_error', title: 'Erro inesperado' };
  return { type: `urn:pp:error:${fallback.code}`, ...fallback, status: res.status, requestId };
}

function isProblem(body: unknown): body is Problem {
  if (typeof body !== 'object' || body === null) return false;
  const p = body as Record<string, unknown>;
  return typeof p.code === 'string' && typeof p.title === 'string';
}
