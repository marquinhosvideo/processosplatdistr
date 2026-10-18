// Seletores do no-restricted-syntax que barram HTML cru (XSS, SDD §2.2): todo
// conteúdo vindo de dado é renderizado como texto. Cobrem também as formas
// indiretas (chave entre aspas, objeto espalhado no JSX, Object.assign,
// acesso computado). Testados em no-raw-html.test.js.

const dangerously = 'dangerouslySetInnerHTML é proibido: renderize o conteúdo como texto.';
const innerHTML = 'innerHTML/outerHTML é proibido: use textContent ou elementos React.';
const rawInsert = 'Inserir HTML cru é proibido: use textContent ou elementos React.';

/** @type {{ selector: string, message: string }[]} */
export const noRawHTML = [
  { selector: "JSXAttribute[name.name='dangerouslySetInnerHTML']", message: dangerously },
  { selector: "Property[key.name='dangerouslySetInnerHTML']", message: dangerously },
  { selector: "Property[key.value='dangerouslySetInnerHTML']", message: dangerously },
  { selector: "MemberExpression[property.name='dangerouslySetInnerHTML']", message: dangerously },
  { selector: "MemberExpression[property.value='dangerouslySetInnerHTML']", message: dangerously },

  { selector: 'AssignmentExpression > MemberExpression.left[property.name=/^(innerHTML|outerHTML)$/]', message: innerHTML },
  { selector: 'AssignmentExpression > MemberExpression.left[property.value=/^(innerHTML|outerHTML)$/]', message: innerHTML },
  { selector: 'Property[key.name=/^(innerHTML|outerHTML)$/]', message: innerHTML },
  { selector: 'Property[key.value=/^(innerHTML|outerHTML)$/]', message: innerHTML },

  { selector: 'CallExpression[callee.property.name=/^(insertAdjacentHTML|setHTMLUnsafe|createContextualFragment)$/]', message: rawInsert },
  { selector: "CallExpression[callee.object.name='document'][callee.property.name=/^(write|writeln)$/]", message: rawInsert },
  { selector: "JSXAttribute[name.name='srcDoc']", message: rawInsert },
];
