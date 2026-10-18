import { screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { RouteObject } from 'react-router';

import { PageLoader } from './components/PageLoader';
import { isChunkLoadError, RouteErrorPage } from './features/errors/RouteErrorPage';
import { AppLayout } from './layout/AppLayout';
import { routes, type RouteHandle } from './router';
import { renderRoute } from './test/render';

function allRoutes(list: RouteObject[]): RouteObject[] {
  return list.flatMap((r) => [r, ...allRoutes(r.children ?? [])]);
}

describe('router', () => {
  it('rota desconhecida mostra "Página não encontrada" dentro da casca', async () => {
    const { user, router } = renderRoute('/qualquer/coisa');

    expect(await screen.findByRole('heading', { level: 1, name: 'Página não encontrada' })).toBeInTheDocument();
    expect(screen.getByRole('navigation', { name: 'Navegação principal' })).toBeInTheDocument();

    await user.click(screen.getByRole('link', { name: 'Voltar ao início' }));
    expect(await screen.findByRole('heading', { level: 1, name: 'Início' })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe('/');
  });

  it('toda página declara título e papéis no handle', () => {
    const pages = allRoutes(routes).filter((r) => r.index === true || r.path === '*' || (r.path !== undefined && r.path !== '/'));
    expect(pages.length).toBeGreaterThan(0);
    for (const page of pages) {
      const handle = page.handle as RouteHandle | undefined;
      expect(handle?.title, page.path ?? 'index').toBeTruthy();
      expect(handle?.roles.length, page.path ?? 'index').toBeGreaterThan(0);
    }
  });

  it('chunk que sumiu depois de um deploy pede para recarregar', async () => {
    const failing: RouteObject[] = [
      {
        path: '/',
        element: <AppLayout />,
        hydrateFallbackElement: <PageLoader />,
        children: [
          {
            errorElement: <RouteErrorPage />,
            children: [
              {
                index: true,
                lazy: () => Promise.reject(new TypeError('Failed to fetch dynamically imported module: /assets/x.js')),
              },
            ],
          },
        ],
      },
    ];
    renderRoute('/', failing);

    expect(await screen.findByRole('heading', { level: 1, name: 'Nova versão disponível' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Recarregar' })).toBeInTheDocument();
  });

  it('erro inesperado de renderização mostra mensagem genérica', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    function Broken(): never {
      throw new Error('boom');
    }
    renderRoute('/', [{ path: '/', element: <AppLayout />, children: [{ errorElement: <RouteErrorPage />, children: [{ index: true, Component: Broken }] }] }]);

    expect(await screen.findByRole('heading', { level: 1, name: 'Algo deu errado' })).toBeInTheDocument();
    expect(screen.queryByText('boom')).not.toBeInTheDocument();
    expect(consoleError).toHaveBeenCalled();
  });

  it('reconhece as mensagens de chunk ausente dos navegadores', () => {
    expect(isChunkLoadError(new TypeError('Failed to fetch dynamically imported module: x'))).toBe(true);
    expect(isChunkLoadError(new TypeError('Importing a module script failed.'))).toBe(true);
    expect(isChunkLoadError(new TypeError('error loading dynamically imported module'))).toBe(true);
    expect(isChunkLoadError(new Error('boom'))).toBe(false);
    expect(isChunkLoadError('Failed to fetch dynamically imported module')).toBe(false);
  });
});
