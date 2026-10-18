// Tipos da API escritos à mão (SDD §8.1), no formato JSON camelCase do servidor.

/** Erro RFC 9457 devolvido por httpx.WriteError (SDD §4.1). */
export interface Problem {
  type: string;
  title: string;
  status: number;
  code: string;
  detail?: string;
  /** Erros por campo do 422 `validation_failed`. */
  errors?: Record<string, string>;
  requestId?: string;
  /** Membros de extensão, ex.: `matches` no 409 `possible_duplicate`. */
  [extension: string]: unknown;
}

/** Envelope das listas paginadas (`?page=&pageSize=`). */
export interface Page<T> {
  items: T[];
  page: number;
  pageSize: number;
  total: number;
}

/** Papéis da sessão (SDD §5.1). Head implica gestor; admin é aditivo. */
export type Role = 'manager' | 'head' | 'admin';

/** Resposta do `GET /healthz`. */
export interface Health {
  status: 'ok';
  version: string;
}
