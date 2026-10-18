import { screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { renderRoute } from '../test/render';

describe('AppLayout', () => {
  it('mostra a casca em pt-BR com cabeçalho, navegação e conteúdo', async () => {
    renderRoute('/');

    expect(await screen.findByRole('heading', { level: 1, name: 'Início' })).toBeInTheDocument();
    expect(within(screen.getByRole('banner')).getByRole('link', { name: 'Ciclo do Colaborador' })).toHaveAttribute('href', '/');
    const nav = screen.getByRole('navigation', { name: 'Navegação principal' });
    expect(within(nav).getByRole('link', { name: 'Início' })).toHaveAttribute('aria-current', 'page');
    expect(screen.getByRole('main')).toHaveAttribute('id', 'conteudo');
    expect(screen.getByRole('link', { name: 'Pular para o conteúdo' })).toHaveAttribute('href', '#conteudo');
  });

  it('põe o título da rota na aba do navegador', async () => {
    renderRoute('/');
    await screen.findByRole('heading', { name: 'Início' });
    await waitFor(() => {
      expect(document.title).toBe('Início · Ciclo do Colaborador');
    });
  });

  it('abre e fecha a navegação pelo botão do menu (celular)', async () => {
    const { user } = renderRoute('/');
    await screen.findByRole('heading', { name: 'Início' });

    const burger = screen.getByRole('button', { name: 'Abrir menu de navegação' });
    expect(burger).toHaveAttribute('aria-expanded', 'false');
    await user.click(burger);
    const close = screen.getByRole('button', { name: 'Fechar menu de navegação' });
    expect(close).toHaveAttribute('aria-expanded', 'true');
    await user.click(close);
    expect(screen.getByRole('button', { name: 'Abrir menu de navegação' })).toHaveAttribute('aria-expanded', 'false');
  });

  it('fecha a navegação do celular ao trocar de página', async () => {
    const { user, router } = renderRoute('/');
    await screen.findByRole('heading', { name: 'Início' });

    await user.click(screen.getByRole('button', { name: 'Abrir menu de navegação' }));
    await router.navigate('/outra');

    expect(await screen.findByRole('heading', { name: 'Página não encontrada' })).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Abrir menu de navegação' })).toHaveAttribute('aria-expanded', 'false');
    });
  });
});
