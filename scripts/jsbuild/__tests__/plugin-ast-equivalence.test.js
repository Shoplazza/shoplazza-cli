'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { assertAstEquivalent } = require('./helpers/ast-equivalent');

test('equivalent code despite formatting/quote differences', () => {
  const a = "const x = 'a';\nfunction f(){ return x }";
  const b = 'const x = "a"; function f() {\n  return x;\n}';
  assertAstEquivalent(a, b); // 不抛错即通过
});

test('detects genuine semantic difference', () => {
  const a = 'const x = 1;';
  const b = 'const x = 2;';
  assert.throws(() => assertAstEquivalent(a, b));
});
