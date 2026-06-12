#!/usr/bin/env node
/**
 * npm bin entry-point — resolves the prebuilt shoplazza binary and forwards
 * all arguments to it transparently.
 */

'use strict';

const { execFileSync } = require('child_process');
const fs   = require('fs');
const path = require('path');

const binName = process.platform === 'win32' ? 'shoplazza.exe' : 'shoplazza';
const binPath = path.join(__dirname, '..', 'bin', binName);

if (!fs.existsSync(binPath)) {
  process.stderr.write(
    `shoplazza binary not found at ${binPath}.\n` +
    `Run 'npm install -g shoplazza-cli' to reinstall, ` +
    `or build from source with 'make install'.\n`
  );
  process.exit(1);
}

try {
  execFileSync(binPath, process.argv.slice(2), { stdio: 'inherit' });
} catch (err) {
  process.exit(err.status ?? 1);
}
