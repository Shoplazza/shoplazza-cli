'use strict';
const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');
const { normalizeBundle } = require('./normalize-bundle');

const BUILD_ENTRY = path.join(__dirname, '..', '..', 'index.js'); // scripts/jsbuild/index.js
const FIXTURES = path.join(__dirname, '..', 'fixtures');

// 用 jsbuild/index.js 实际构建 fixture，返回归一化后的单一产物源码。
// index.js 从 stdin 读 {name}，以 process.cwd() 为根；用 cwd 选项指定 fixture 根。
function buildFixture(fixtureName, extId = 'demo') {
  const cwd = path.join(FIXTURES, fixtureName);
  const dist = path.join(cwd, 'dist');
  fs.rmSync(dist, { recursive: true, force: true });

  execFileSync('node', [BUILD_ENTRY], {
    cwd,
    input: JSON.stringify({ name: extId }),
    stdio: ['pipe', 'pipe', 'inherit'],
  });

  const files = fs.readdirSync(dist).filter((f) => f.startsWith(extId + '.') && f.endsWith('.js'));
  if (files.length !== 1) {
    throw new Error('expected exactly 1 bundle for ' + fixtureName + ', got: ' + files.join(', '));
  }
  const out = normalizeBundle(fs.readFileSync(path.join(dist, files[0]), 'utf8'));
  fs.rmSync(dist, { recursive: true, force: true });
  return out;
}

module.exports = { buildFixture };
