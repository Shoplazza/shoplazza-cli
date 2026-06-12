'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const acorn = require('acorn');
const { htmlToJsStringLiteral } = require('../plugin/html-string-literal');

// Parse `x = <literal>` and read back the string literal value; verify embed->parse is identity
function roundTrip(s) {
  const code = 'var x = ' + htmlToJsStringLiteral(s) + ';';
  const ast = acorn.parse(code, { ecmaVersion: 'latest', sourceType: 'module' });
  const decl = ast.body[0].declarations[0].init;
  assert.equal(decl.type, 'Literal', 'embedded node must be a string literal');
  assert.equal(typeof decl.value, 'string');
  return decl.value;
}

const ADVERSARIAL = {
  'double quotes': '<a title="hi">x</a>',
  'single quotes': "<a title='hi'>x</a>",
  'backtick + template': '<b>`${danger}`</b>',
  'closing script tag': '<div></div><script>alert(1)</script>',
  'backslash': 'C:\\path\\to\\file and a literal \\n',
  'newlines + tabs': 'line1\n\tline2\r\nline3',
  'unicode + emoji': '<p>你好 🌮 café</p>',
  'line separators U+2028/U+2029': 'a\u2028b\u2029c',
  'NUL and control': 'x\x00yz',
  'empty': '',
};

for (const [name, input] of Object.entries(ADVERSARIAL)) {
  test('round-trips: ' + name, () => {
    assert.equal(roundTrip(input), input);
  });
}
