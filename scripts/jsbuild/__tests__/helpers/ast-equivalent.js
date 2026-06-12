'use strict';
const assert = require('node:assert/strict');
const acorn = require('acorn');

// 去掉只影响"长相"不影响"语义"的字段：位置、原始文本、注释。
const DROP = new Set(['start', 'end', 'loc', 'range', 'raw', 'comments']);

function normalize(node) {
  if (Array.isArray(node)) return node.map(normalize);
  if (node && typeof node === 'object') {
    const out = {};
    for (const k of Object.keys(node).sort()) {
      if (DROP.has(k)) continue;
      out[k] = normalize(node[k]);
    }
    return out;
  }
  return node;
}

function parse(code) {
  return normalize(
    acorn.parse(code, { ecmaVersion: 'latest', sourceType: 'module' })
  );
}

function assertAstEquivalent(codeA, codeB) {
  assert.deepEqual(parse(codeA), parse(codeB));
}

module.exports = { assertAstEquivalent, parse };
