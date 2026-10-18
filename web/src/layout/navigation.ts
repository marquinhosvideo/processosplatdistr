import { IconHome, type Icon } from '@tabler/icons-react';

import type { Role } from '../api/types';

export interface NavItem {
  to: string;
  label: string;
  icon: Icon;
  /** Papéis que veem o item; a navegação é filtrada por papel (SDD §8.2). */
  roles: readonly Role[];
}

// Os módulos entram aqui conforme chegam nos marcos seguintes.
export const navItems: readonly NavItem[] = [{ to: '/', label: 'Início', icon: IconHome, roles: ['manager'] }];
