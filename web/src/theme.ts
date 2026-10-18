import {
  createTheme,
  darken,
  DEFAULT_THEME,
  luminance,
  toRgba,
  type CSSVariablesResolver,
  type MantineColorSchemeManager,
  type MantineColorsTuple,
} from '@mantine/core';

// Contraste mínimo de texto normal no WCAG 2.x AA (critério 1.4.3, RNF-12).
export const MIN_CONTRAST = 4.5;

const WHITE = '#ffffff';
// Fundo neutro mais escuro usado sob texto (hover, linhas zebradas).
const NEUTRAL_BG = DEFAULT_THEME.colors.gray[1];
// Opacidade máxima do fundo da variante "light" (light-hover do Mantine).
const LIGHT_BG_ALPHA = 0.12;

export function contrastRatio(a: string, b: string): number {
  const la = luminance(a);
  const lb = luminance(b);
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
}

/** Cor sólida equivalente a `color` com opacidade `alpha` sobre branco. */
export function overWhite(color: string, alpha: number): string {
  const { r, g, b } = toRgba(color);
  const mix = (c: number) => Math.round(c * alpha + 255 * (1 - alpha));
  return `rgb(${mix(r)}, ${mix(g)}, ${mix(b)})`;
}

function toHex(color: string): string {
  const { r, g, b } = toRgba(color);
  return '#' + [r, g, b].map((c) => Math.round(c).toString(16).padStart(2, '0')).join('');
}

/** Escurece `color` aos poucos até satisfazer `ok`. */
function darkenUntil(color: string, ok: (c: string) => boolean): string {
  let current = color;
  for (let step = 1; !ok(current) && step <= 50; step++) {
    current = toHex(darken(color, step * 0.02));
  }
  return current;
}

/**
 * Ajusta uma paleta do Mantine para o tema claro passar no AA. Várias cores
 * padrão (verde, laranja, amarelo, ciano…) ficam abaixo de 4,5:1 nas
 * tonalidades usadas como texto. Aqui:
 * - a 8 (preenchimento com texto branco, texto colorido e outline) atinge
 *   4,5:1 sobre o fundo neutro;
 * - a 9 (texto da variante light e hover do preenchimento) atinge 4,5:1 sobre
 *   o fundo translúcido da própria cor.
 */
export function accessiblePalette(colors: MantineColorsTuple): MantineColorsTuple {
  const shades: string[] = [...colors];
  const shade8 = darkenUntil(colors[8], (c) => contrastRatio(c, NEUTRAL_BG) >= MIN_CONTRAST);
  const lightBg = overWhite(shade8, LIGHT_BG_ALPHA);
  shades[8] = shade8;
  shades[9] = darkenUntil(
    colors[9],
    (c) => contrastRatio(c, lightBg) >= MIN_CONTRAST && luminance(c) <= luminance(shade8),
  );
  return shades as unknown as MantineColorsTuple;
}

const palettes = Object.fromEntries(
  Object.entries(DEFAULT_THEME.colors).map(([name, colors]) => [name, accessiblePalette(colors)]),
);

// Texto auxiliar (dimmed) e placeholder também precisam de 4,5:1: os padrões
// do Mantine (gray-6 e gray-5) ficam em ~3,3:1 e ~2:1.
export const DIMMED_COLOR = DEFAULT_THEME.colors.gray[7];
export const PLACEHOLDER_COLOR = darkenUntil(
  DEFAULT_THEME.colors.gray[6],
  (c) => contrastRatio(c, WHITE) >= MIN_CONTRAST,
);

export const theme = createTheme({
  colors: palettes,
  primaryColor: 'blue',
  primaryShade: { light: 8, dark: 8 },
  autoContrast: true,
  defaultRadius: 'sm',
  cursorType: 'pointer',
  components: {
    // Link não pode depender só da cor para ser identificado (WCAG 1.4.1).
    Anchor: { defaultProps: { underline: 'always' } },
  },
});

export const cssVariablesResolver: CSSVariablesResolver = (t) => ({
  variables: {},
  light: {
    '--mantine-color-dimmed': DIMMED_COLOR,
    '--mantine-color-placeholder': PLACEHOLDER_COLOR,
    '--mantine-color-error': 'var(--mantine-color-red-8)',
    ...Object.fromEntries(
      Object.keys(t.colors).map((name) => [`--mantine-color-${name}-light-color`, `var(--mantine-color-${name}-9)`]),
    ),
  },
  dark: {},
});

/**
 * O app só tem o tema claro. Este gerenciador não lê nem grava localStorage
 * (o padrão do Mantine grava) e dispensa o ColorSchemeScript, que é script
 * inline e violaria a CSP.
 */
export const lightColorScheme: MantineColorSchemeManager = {
  get: () => 'light',
  set: () => undefined,
  subscribe: () => undefined,
  unsubscribe: () => undefined,
  clear: () => undefined,
};
