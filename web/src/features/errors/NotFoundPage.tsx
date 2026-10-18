import { Button } from '@mantine/core';
import { Link } from 'react-router';

import { EmptyState } from '../../components/EmptyState';
import { PageHeader } from '../../components/PageHeader';

export function NotFoundPage() {
  return (
    <>
      <PageHeader title="Página não encontrada" />
      <EmptyState
        title="Não encontramos este endereço"
        description="O link pode estar errado ou a página pode ter sido removida."
        action={
          <Button component={Link} to="/" mt="sm">
            Voltar ao início
          </Button>
        }
      />
    </>
  );
}
