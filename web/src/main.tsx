import '@mantine/core/styles.css';
import '@mantine/dates/styles.css';
import '@mantine/notifications/styles.css';

import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { createBrowserRouter } from 'react-router';
import { RouterProvider } from 'react-router/dom';

import { AppProviders } from './App';
import { createQueryClient } from './api/queryClient';
import { routes } from './router';

const container = document.getElementById('root');
if (!container) throw new Error('elemento #root ausente no index.html');

const queryClient = createQueryClient();
const router = createBrowserRouter(routes);

createRoot(container).render(
  <StrictMode>
    <AppProviders queryClient={queryClient}>
      <RouterProvider router={router} />
    </AppProviders>
  </StrictMode>,
);
