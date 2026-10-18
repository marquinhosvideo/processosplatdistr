import type { RouteObject } from 'react-router';

import type { Role } from './api/types';
import { PageLoader } from './components/PageLoader';
import { NotFoundPage } from './features/errors/NotFoundPage';
import { RouteErrorPage } from './features/errors/RouteErrorPage';
import { AppLayout } from './layout/AppLayout';

/** Metadados de cada rota: título da aba e papéis que podem abri-la. */
export interface RouteHandle {
  title: string;
  roles: readonly Role[];
}

const G: readonly Role[] = ['manager'];

function handle(title: string, roles: readonly Role[]): RouteHandle {
  return { title, roles };
}

// Rotas lazy (SDD §8.2): cada página vira um chunk carregado sob demanda.
export const routes: RouteObject[] = [
  {
    path: '/',
    element: <AppLayout />,
    errorElement: <RouteErrorPage />,
    // Exibido só na primeira carga, enquanto o chunk lazy da rota inicial chega.
    hydrateFallbackElement: <PageLoader />,
    children: [
      {
        // Erros das páginas aparecem dentro da casca, com a navegação.
        errorElement: <RouteErrorPage />,
        children: [
          {
            index: true,
            handle: handle('Início', G),
            lazy: async () => ({ Component: (await import('./features/home/HomePage')).HomePage }),
          },
          { path: '*', handle: handle('Página não encontrada', G), Component: NotFoundPage },
        ],
      },
    ],
  },
];
