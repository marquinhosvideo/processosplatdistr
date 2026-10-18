import { Button } from '@mantine/core';
import { isRouteErrorResponse, useRouteError } from 'react-router';

import { EmptyState } from '../../components/EmptyState';
import { PageHeader } from '../../components/PageHeader';
import { NotFoundPage } from './NotFoundPage';

// Mensagens dos navegadores quando um chunk lazy some depois de um deploy.
const CHUNK_ERROR = /dynamically imported module|Importing a module script failed|error loading dynamically imported module/i;

export function isChunkLoadError(error: unknown): boolean {
  return error instanceof Error && CHUNK_ERROR.test(error.message);
}

function reload() {
  window.location.reload();
}

/** errorElement das rotas: falha de carregamento ou erro inesperado de renderização. */
export function RouteErrorPage() {
  const error = useRouteError();
  if (isRouteErrorResponse(error) && error.status === 404) return <NotFoundPage />;

  const outdated = isChunkLoadError(error);
  if (!outdated) console.error(error);
  return (
    <>
      <PageHeader title={outdated ? 'Nova versão disponível' : 'Algo deu errado'} />
      <EmptyState
        title={outdated ? 'A aplicação foi atualizada' : 'Não foi possível mostrar esta página'}
        description={
          outdated
            ? 'Recarregue a página para continuar com a versão mais recente.'
            : 'Tente recarregar a página. Se o problema continuar, avise o time da plataforma.'
        }
        action={
          <Button onClick={reload} mt="sm">
            Recarregar
          </Button>
        }
      />
    </>
  );
}
