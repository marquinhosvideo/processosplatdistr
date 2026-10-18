import { DEFAULT_THEME } from '@mantine/core';
import { describe, expect, it } from 'vitest';

import { contrastRatio, DIMMED_COLOR, MIN_CONTRAST, overWhite, PLACEHOLDER_COLOR, theme } from './theme';

const WHITE = '#ffffff';
const NEUTRAL_BG = DEFAULT_THEME.colors.gray[1];

describe('contraste AA do tema', () => {
  it('calcula a razão de contraste do WCAG', () => {
    expect(contrastRatio('#000000', '#ffffff')).toBeCloseTo(21, 5);
    expect(contrastRatio('#ffffff', '#ffffff')).toBeCloseTo(1, 5);
    expect(contrastRatio('#767676', '#ffffff')).toBeCloseTo(4.54, 2);
  });

  it('usa a tonalidade 8 como cor primária no tema claro', () => {
    expect(theme.primaryShade).toEqual({ light: 8, dark: 8 });
    expect(theme.autoContrast).toBe(true);
  });

  const colors = Object.entries(theme.colors ?? {}).flatMap(([name, shades]) => (shades ? [[name, shades] as const] : []));

  it('ajusta todas as paletas do Mantine', () => {
    expect(colors.map(([name]) => name).sort()).toEqual(Object.keys(DEFAULT_THEME.colors).sort());
  });

  it.each(colors)('%s: preenchimento, texto e variante light passam no AA', (_name, shades) => {
    const filled = shades[8];
    const lightText = shades[9];
    expect(contrastRatio(filled, WHITE)).toBeGreaterThanOrEqual(MIN_CONTRAST);
    expect(contrastRatio(filled, NEUTRAL_BG)).toBeGreaterThanOrEqual(MIN_CONTRAST);
    expect(contrastRatio(lightText, overWhite(filled, 0.12))).toBeGreaterThanOrEqual(MIN_CONTRAST);
    expect(contrastRatio(lightText, WHITE)).toBeGreaterThanOrEqual(contrastRatio(filled, WHITE));
  });

  it('texto auxiliar e placeholder passam no AA sobre branco e fundo neutro', () => {
    expect(contrastRatio(DIMMED_COLOR, WHITE)).toBeGreaterThanOrEqual(MIN_CONTRAST);
    expect(contrastRatio(DIMMED_COLOR, NEUTRAL_BG)).toBeGreaterThanOrEqual(MIN_CONTRAST);
    expect(contrastRatio(PLACEHOLDER_COLOR, WHITE)).toBeGreaterThanOrEqual(MIN_CONTRAST);
  });

  it('não escurece além do necessário as cores que já passavam', () => {
    expect(theme.colors?.blue?.[8]).toBe(DEFAULT_THEME.colors.blue[8]);
    expect(theme.colors?.gray).toEqual(DEFAULT_THEME.colors.gray);
  });
});
