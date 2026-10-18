import { Group, Stack, Text, Title } from '@mantine/core';
import type { ReactNode } from 'react';

interface PageHeaderProps {
  title: string;
  description?: ReactNode;
  /** Botões de ação, alinhados à direita. */
  actions?: ReactNode;
}

/** Título da página: o único h1, que dá nome ao conteúdo principal. */
export function PageHeader({ title, description, actions }: PageHeaderProps) {
  return (
    <Group justify="space-between" align="flex-start" mb="lg" wrap="wrap" gap="sm">
      <Stack gap={4}>
        <Title order={1} size="h2">
          {title}
        </Title>
        {description !== undefined && <Text c="dimmed">{description}</Text>}
      </Stack>
      {actions !== undefined && <Group gap="sm">{actions}</Group>}
    </Group>
  );
}
