import { ESLint } from 'eslint';
import tseslint from 'typescript-eslint';
import { describe, expect, it } from 'vitest';

import { noRawHTML } from './no-raw-html.js';

// Só a regra em teste, sem type-check: o trecho é analisado como um .tsx solto.
const eslint = new ESLint({
  overrideConfigFile: true,
  overrideConfig: [
    {
      files: ['**/*.tsx'],
      languageOptions: { parser: tseslint.parser, parserOptions: { ecmaFeatures: { jsx: true } } },
      rules: { 'no-restricted-syntax': ['error', ...noRawHTML] },
    },
  ],
});

const prelude = 'declare const el: HTMLElement; declare const html: string; declare const range: Range;\n';

/** @param {string} code */
async function violations(code) {
  const [result] = await eslint.lintText(prelude + code, { filePath: 'probe.tsx' });
  const fatal = result.messages.filter((m) => m.fatal);
  if (fatal.length > 0) throw new Error(`trecho não analisou: ${fatal[0].message}`);
  return result.messages.filter((m) => m.ruleId === 'no-restricted-syntax').length;
}

describe('no-raw-html', () => {
  it.each([
    ['atributo JSX', 'export const A = () => <div dangerouslySetInnerHTML={{ __html: html }} />;'],
    ['chave entre aspas espalhada no JSX', "const p = { 'dangerouslySetInnerHTML': { __html: html } };\nexport const A = () => <div {...p} />;"],
    ['chave abreviada', 'const dangerouslySetInnerHTML = { __html: html };\nexport const p = { dangerouslySetInnerHTML };'],
    ['acesso computado', "const p: Record<string, unknown> = {};\np['dangerouslySetInnerHTML'] = { __html: html };"],
    ['innerHTML', 'el.innerHTML = html;'],
    ['innerHTML +=', 'el.innerHTML += html;'],
    ['outerHTML computado', "el['outerHTML'] = html;"],
    ['Object.assign', 'Object.assign(el, { innerHTML: html });'],
    ['Object.assign com aspas', "Object.assign(el, { 'outerHTML': html });"],
    ['insertAdjacentHTML', "el.insertAdjacentHTML('beforeend', html);"],
    ['createContextualFragment', 'range.createContextualFragment(html);'],
    ['setHTMLUnsafe', "(el as unknown as { setHTMLUnsafe(h: string): void }).setHTMLUnsafe(html);"],
    ['document.write', 'document.write(html);'],
    ['iframe srcDoc', 'export const A = () => <iframe srcDoc={html} />;'],
  ])('recusa %s', async (_name, code) => {
    expect(await violations(code)).toBeGreaterThan(0);
  });

  it('aceita texto e leituras', async () => {
    const code = [
      'el.textContent = html;',
      "el.insertAdjacentText('beforeend', html);",
      'export const lido = el.innerHTML;',
      'export const A = () => <p title={html}>{html}</p>;',
      'declare const stream: { write(s: string): void };',
      'stream.write(html);',
    ].join('\n');
    expect(await violations(code)).toBe(0);
  });
});
