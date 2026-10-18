import { AppShell, Burger, Group, NavLink, Text } from '@mantine/core';
import { useDisclosure } from '@mantine/hooks';
import { useEffect } from 'react';
import { Link, matchPath, NavLink as RouterNavLink, Outlet, useLocation, useMatches } from 'react-router';

import type { RouteHandle } from '../router';
import classes from './AppLayout.module.css';
import { navItems } from './navigation';

export const APP_NAME = 'Ciclo do Colaborador';

function useDocumentTitle() {
  const matches = useMatches();
  const title = matches
    .map((m) => (m.handle as RouteHandle | undefined)?.title)
    .filter((t): t is string => t !== undefined)
    .at(-1);
  useEffect(() => {
    document.title = title ? `${title} · ${APP_NAME}` : APP_NAME;
  }, [title]);
}

/** Casca do SPA: cabeçalho, navegação lateral (gaveta no celular) e conteúdo. */
export function AppLayout() {
  const [navOpened, { toggle: toggleNav, close: closeNav }] = useDisclosure(false);
  const { pathname } = useLocation();
  useDocumentTitle();

  // No celular, a gaveta fecha ao navegar.
  useEffect(() => {
    closeNav();
  }, [pathname, closeNav]);

  return (
    <AppShell
      header={{ height: 56 }}
      navbar={{ width: 248, breakpoint: 'sm', collapsed: { mobile: !navOpened } }}
      padding="md"
    >
      <a href="#conteudo" className={classes.skipLink}>
        Pular para o conteúdo
      </a>
      <AppShell.Header>
        <Group h="100%" px="md" gap="sm">
          <Burger
            opened={navOpened}
            onClick={toggleNav}
            hiddenFrom="sm"
            size="sm"
            aria-label={navOpened ? 'Fechar menu de navegação' : 'Abrir menu de navegação'}
            aria-expanded={navOpened}
            aria-controls="navegacao-principal"
          />
          <Text component={Link} to="/" fw={700} size="lg" className={classes.brand}>
            {APP_NAME}
          </Text>
        </Group>
      </AppShell.Header>
      <AppShell.Navbar id="navegacao-principal" p="xs" aria-label="Navegação principal">
        {navItems.map((item) => (
          <NavLink
            key={item.to}
            component={RouterNavLink}
            to={item.to}
            end={item.to === '/'}
            label={item.label}
            leftSection={<item.icon size={18} stroke={1.6} aria-hidden />}
            active={matchPath({ path: item.to, end: item.to === '/' }, pathname) !== null}
          />
        ))}
      </AppShell.Navbar>
      <AppShell.Main id="conteudo" tabIndex={-1}>
        <Outlet />
      </AppShell.Main>
    </AppShell>
  );
}
