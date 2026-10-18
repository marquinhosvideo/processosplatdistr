import { render } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactElement } from 'react';
import { createMemoryRouter, RouterProvider, type RouteObject } from 'react-router';

import { AppProviders } from '../App';
import { createQueryClient } from '../api/queryClient';
import { routes as appRoutes } from '../router';

function testQueryClient() {
  const client = createQueryClient();
  client.setDefaultOptions({ queries: { retry: false, gcTime: Infinity }, mutations: { retry: false } });
  return client;
}

/** Renderiza um componente solto com os provedores do app. */
export function renderWithProviders(ui: ReactElement) {
  const queryClient = testQueryClient();
  return {
    user: userEvent.setup(),
    queryClient,
    ...render(
      <AppProviders queryClient={queryClient} env="test">
        {ui}
      </AppProviders>,
    ),
  };
}

/** Renderiza o app (ou outras rotas) a partir de `path`, com router em memória. */
export function renderRoute(path = '/', routes: RouteObject[] = appRoutes) {
  const router = createMemoryRouter(routes, { initialEntries: [path] });
  return { router, ...renderWithProviders(<RouterProvider router={router} />) };
}
