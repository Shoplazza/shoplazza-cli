#!/usr/bin/env node
/**
 * postinstall script — downloads the prebuilt shoplazza binary for the current
 * platform from GitHub Releases and places it at bin/shoplazza[.exe].
 *
 * Environment overrides:
 *   SHOPLAZZA_CLI_RELEASE_URL  — custom base URL (default: GitHub release asset URL)
 *   SHOPLAZZA_CLI_SKIP_INSTALL — set to "1" to skip download (useful for CI that
 *                                 builds from source instead)
 */

'use strict';

const { execSync } = require('child_process');
const fs = require('fs');
const https = require('https');
const http = require('http');
const os = require('os');
const path = require('path');
const { version } = require('../package.json');

if (process.env.SHOPLAZZA_CLI_SKIP_INSTALL === '1') {
  console.log('SHOPLAZZA_CLI_SKIP_INSTALL=1 — skipping binary download.');
  process.exit(0);
}

// ── Platform detection ────────────────────────────────────────────────────────

const PLATFORM_MAP = {
  darwin:  'darwin',
  linux:   'linux',
  win32:   'windows',
};

const ARCH_MAP = {
  x64:   'amd64',
  arm64: 'arm64',
};

const platform = PLATFORM_MAP[process.platform];
const arch     = ARCH_MAP[process.arch];

if (!platform || !arch) {
  console.error(`Unsupported platform: ${process.platform}/${process.arch}`);
  console.error('Please build from source: https://github.com/Shoplazza/shoplazza-cli');
  process.exit(1);
}

const ext      = platform === 'windows' ? '.zip' : '.tar.gz';
const binName  = platform === 'windows' ? 'shoplazza.exe' : 'shoplazza';
const pkgName  = `shoplazza-cli-${version}-${platform}-${arch}`;
const fileName = `${pkgName}${ext}`;

// ── Destination ───────────────────────────────────────────────────────────────

const binDir  = path.join(__dirname, '..', 'bin');
const binPath = path.join(binDir, binName);

fs.mkdirSync(binDir, { recursive: true });

// ── Download URL ──────────────────────────────────────────────────────────────

const RELEASE_BASE =
  process.env.SHOPLAZZA_CLI_RELEASE_URL ||
  `https://github.com/Shoplazza/shoplazza-cli/releases/download/v${version}`;

const downloadURL = `${RELEASE_BASE}/${fileName}`;

// ── Helpers ───────────────────────────────────────────────────────────────────

function download(url, dest) {
  return new Promise((resolve, reject) => {
    function get(u) {
      const proto = u.startsWith('https') ? https : http;
      proto.get(u, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          res.resume(); // Drain response before following redirect.
          get(res.headers.location);
          return;
        }
        if (res.statusCode !== 200) {
          res.resume();
          reject(new Error(`HTTP ${res.statusCode} for ${u}`));
          return;
        }
        const file = fs.createWriteStream(dest);
        res.pipe(file);
        file.on('finish', () => file.close(resolve));
        file.on('error', (err) => {
          fs.unlink(dest, () => {});
          reject(err);
        });
      }).on('error', (err) => {
        fs.unlink(dest, () => {});
        reject(err);
      });
    }

    get(url);
  });
}

function extract(archive, destDir) {
  if (archive.endsWith('.tar.gz')) {
    execSync(`tar -xzf ${JSON.stringify(archive)} -C ${JSON.stringify(destDir)}`);
  } else {
    // .zip (Windows)
    execSync(`powershell -Command "Expand-Archive -Path '${archive}' -DestinationPath '${destDir}' -Force"`);
  }
}

// ── Main ──────────────────────────────────────────────────────────────────────

(async () => {
  const tmpDir     = fs.mkdtempSync(path.join(os.tmpdir(), 'shoplazza-install-'));
  const archivePath = path.join(tmpDir, fileName);

  try {
    console.log(`Downloading ${fileName} …`);
    await download(downloadURL, archivePath);

    console.log(`Extracting …`);
    extract(archivePath, tmpDir);

    // Binary is inside a named subdirectory matching the archive name.
    const extracted = path.join(tmpDir, pkgName, binName);
    const fallback  = path.join(tmpDir, binName);
    const src       = fs.existsSync(extracted) ? extracted : fallback;

    fs.copyFileSync(src, binPath);
    if (platform !== 'windows') {
      fs.chmodSync(binPath, 0o755);
    }

    console.log(`Installed shoplazza ${version} → ${binPath}`);
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
})().catch((err) => {
  console.error(`Failed to install shoplazza CLI: ${err.message}`);
  console.error('You can build from source instead:');
  console.error('  git clone https://github.com/Shoplazza/shoplazza-cli.git');
  console.error('  cd shoplazza-cli && make install');
  process.exit(1);
});
