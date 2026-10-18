import { IconChecklist } from '@tabler/icons-react';

import { EmptyState } from '../../components/EmptyState';
import { PageHeader } from '../../components/PageHeader';

export function HomePage() {
  return (
    <>
      <PageHeader
        title="Início"
        description="Recrutamento, onboarding, Nine Box e treinamentos da gerência em um só lugar."
      />
      <EmptyState
        icon={<IconChecklist size={26} stroke={1.6} />}
        title="Nenhuma pendência para mostrar"
        description="As pendências de cada módulo aparecem aqui assim que os módulos forem liberados."
      />
    </>
  );
}
