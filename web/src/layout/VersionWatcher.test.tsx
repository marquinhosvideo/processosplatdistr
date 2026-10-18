import { screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { request } from '../api/client';
import { renderWithProviders } from '../test/render';

describe('VersionWatcher', () => {
  it('oferece recarregar quando a X-App-Version muda', async () => {
    const reply = (v: string) =>
      Promise.resolve(new Response('{}', { status: 200, headers: { 'Content-Type': 'application/json', 'X-App-Version': v } }));
    vi.stubGlobal('fetch', vi.fn().mockImplementationOnce(() => reply('v1')).mockImplementationOnce(() => reply('v2')));
    renderWithProviders(<p>app</p>);

    await request('/api/v1/x');
    expect(screen.queryByText('Nova versão disponível')).not.toBeInTheDocument();

    await request('/api/v1/x');
    expect(await screen.findByText('Nova versão disponível')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Recarregar' })).toBeInTheDocument();
  });
});
