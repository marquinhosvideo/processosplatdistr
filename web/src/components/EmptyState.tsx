import { Paper, Stack, Text, ThemeIcon } from '@mantine/core';
import type { ReactNode } from 'react';

interface EmptyStateProps {
  title: string;
  description?: ReactNode;
  icon?: ReactNode;
  /** Ação sugerida (ex.: botão "Cadastrar"). */
  action?: ReactNode;
}

export function EmptyState({ title, description, icon, action }: EmptyStateProps) {
  return (
    <Paper withBorder p="xl" radius="md">
      <Stack align="center" gap="xs" ta="center">
        {icon !== undefined && (
          <ThemeIcon variant="light" size={48} radius="xl" aria-hidden>
            {icon}
          </ThemeIcon>
        )}
        <Text fw={600}>{title}</Text>
        {description !== undefined && <Text c="dimmed" size="sm">{description}</Text>}
        {action}
      </Stack>
    </Paper>
  );
}
