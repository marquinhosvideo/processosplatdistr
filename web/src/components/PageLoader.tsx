import { Center, Loader } from '@mantine/core';

/** Indicador enquanto o chunk da página carrega. */
export function PageLoader() {
  return (
    <Center mih="40vh" role="status" aria-live="polite">
      <Loader aria-label="Carregando" />
    </Center>
  );
}
