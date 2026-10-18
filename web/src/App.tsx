import { MantineProvider } from '@mantine/core';
import { DatesProvider } from '@mantine/dates';
import { Notifications } from '@mantine/notifications';
import { QueryClientProvider, type QueryClient } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import 'dayjs/locale/pt-br';

import { cssVariablesResolver, lightColorScheme, theme } from './theme';
import { VersionWatcher } from './layout/VersionWatcher';

interface AppProvidersProps {
  queryClient: QueryClient;
  children: ReactNode;
  /** "test" desliga transições e portais do Mantine nos testes de componente. */
  env?: 'default' | 'test';
}

/** Provedores globais do SPA (SDD §8.1), compartilhados com os testes. */
export function AppProviders({ queryClient, children, env = 'default' }: AppProvidersProps) {
  return (
    <MantineProvider
      theme={theme}
      cssVariablesResolver={cssVariablesResolver}
      colorSchemeManager={lightColorScheme}
      forceColorScheme="light"
      env={env}
    >
      <DatesProvider settings={{ locale: 'pt-br', firstDayOfWeek: 0, weekendDays: [0, 6] }}>
        <Notifications position="top-right" limit={3} />
        <VersionWatcher />
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      </DatesProvider>
    </MantineProvider>
  );
}
