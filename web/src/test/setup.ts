import '@testing-library/jest-dom/vitest';

import { cleanup } from '@testing-library/react';
import { afterEach, beforeEach } from 'vitest';

import { resetClientState } from '../api/client';

// O jsdom não implementa estas APIs, que o Mantine usa (useMediaQuery,
// ScrollArea, Popover…).
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: (query: string): MediaQueryList => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => undefined,
    removeListener: () => undefined,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    dispatchEvent: () => false,
  }),
});

class ResizeObserverStub implements ResizeObserver {
  observe() {
    return undefined;
  }
  unobserve() {
    return undefined;
  }
  disconnect() {
    return undefined;
  }
}
window.ResizeObserver = ResizeObserverStub;

window.HTMLElement.prototype.scrollIntoView = () => undefined;
window.scrollTo = () => undefined;

beforeEach(() => {
  resetClientState();
});

afterEach(() => {
  cleanup();
});
