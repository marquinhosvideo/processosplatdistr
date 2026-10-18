import { Button, Stack, Text } from '@mantine/core';
import { notifications } from '@mantine/notifications';
import { useEffect } from 'react';

import { onVersionChange } from '../api/client';

export const VERSION_NOTIFICATION_ID = 'app-version';

/** Oferece recarregar o SPA quando o servidor muda de versão (deploy). */
export function VersionWatcher() {
  useEffect(
    () =>
      onVersionChange(() => {
        notifications.show({
          id: VERSION_NOTIFICATION_ID,
          title: 'Nova versão disponível',
          message: (
            <Stack gap="xs" align="flex-start">
              <Text size="sm">Recarregue a página para usar a versão mais recente.</Text>
              <Button size="xs" onClick={() => { window.location.reload(); }}>
                Recarregar
              </Button>
            </Stack>
          ),
          autoClose: false,
        });
      }),
    [],
  );
  return null;
}
