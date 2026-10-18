import { QueryClient } from '@tanstack/react-query';

import { ApiError } from './client';

const MAX_RETRIES = 2;

/** Erros do cliente (4xx) não melhoram com nova tentativa; rede e 5xx sim. */
export function shouldRetry(failureCount: number, error: unknown): boolean {
  if (error instanceof ApiError && error.status >= 400 && error.status < 500) return false;
  return failureCount < MAX_RETRIES;
}

/**
 * Cache só em memória (SDD §2.2): nada é persistido no navegador e o logout
 * chama queryClient.clear().
 */
export function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        retry: shouldRetry,
        refetchOnWindowFocus: false,
      },
      mutations: { retry: false },
    },
  });
}
