'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { buildFixture } = require('./helpers/build-fixture');

const goldenDir = path.join(__dirname, 'golden');

for (const name of ['basic', 'escaping', 'nested', 'apiprefix']) {
  test('bundle matches golden: ' + name, () => {
    const expected = fs.readFileSync(path.join(goldenDir, name + '.bundle.js'), 'utf8');
    const actual = buildFixture(name);
    assert.equal(actual, expected,
      `产物与 golden 不一致（fixture=${name}）。若确属预期变化，重跑 capture 步骤更新 golden。`);
  });
}
