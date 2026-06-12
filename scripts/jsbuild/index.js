#!/usr/bin/env node
'use strict';
const fs = require('fs');
const path = require('path');
const { build } = require('vite');
const { viteConfig } = require('./vite.config');

function getRootPath() {
  return process.cwd(); // inherited from the Go parent (= user invocation dir, = v1 process.cwd())
}
function getDistPath() {
  return path.join(getRootPath(), 'dist');
}

async function runBuild(req) {
  const id = req.name;
  if (!id) throw new Error('name is required');
  const entry = { [id]: path.join(getRootPath(), 'extensions', id, 'src', 'index.js') };
  const distPath = getDistPath();
  if (!fs.existsSync(distPath)) fs.mkdirSync(distPath, { recursive: true });
  // Match only THIS id's artifacts ("foo" or "foo.<hash>.js") — a bare
  // includes(id) also clears/collects sibling extensions like "foo-bar".
  const matchesId = (fname) => fname === id || fname.startsWith(id + '.');
  for (const fname of fs.readdirSync(distPath)) {
    if (matchesId(fname)) fs.rmSync(path.join(distPath, fname)); // clear prior dist for this id (v1 build.js:18-22)
  }
  const started = Date.now();
  await build(viteConfig(entry));
  const artifacts = fs
    .readdirSync(distPath)
    .filter(matchesId)
    .map((f) => path.join('dist', f));
  return { ok: true, artifacts, durationMs: Date.now() - started };
}

function readStdin() {
  return new Promise((resolve, reject) => {
    let buf = '';
    process.stdin.setEncoding('utf8');
    process.stdin.on('data', (c) => (buf += c));
    process.stdin.on('end', () => resolve(buf));
    process.stdin.on('error', reject);
  });
}

(async () => {
  let req;
  try {
    req = JSON.parse((await readStdin()).trim());
  } catch (err) {
    process.stdout.write(
      JSON.stringify({ ok: false, error: { type: 'internal', code: 'bad_request', message: 'invalid request JSON: ' + err.message } }) + '\n',
      () => process.exit(1) // flush before exit (match the happy path)
    );
    return;
  }
  // Protect the single-line JSON stdio protocol: Vite/Rollup log to stdout via
  // console (e.g. "vite v4.3.9 building...", "Generated an empty chunk"). Route
  // ALL stdout writes to stderr during the build so stdout carries ONLY our final
  // JSON result line; emit() restores stdout and writes the JSON, flushing before exit.
  const realStdoutWrite = process.stdout.write.bind(process.stdout);
  process.stdout.write = process.stderr.write.bind(process.stderr);
  const emit = (obj, code) => {
    process.stdout.write = realStdoutWrite;
    realStdoutWrite(JSON.stringify(obj) + '\n', () => process.exit(code));
  };

  if (req.debug) process.stderr.write('[checkout build] request: ' + JSON.stringify(req) + '\n');
  try {
    const result = await runBuild(req);
    if (req.debug) process.stderr.write('[checkout build] artifacts: ' + JSON.stringify(result.artifacts) + '\n');
    emit(result, 0);
  } catch (err) {
    if (req.debug) process.stderr.write(String(err && err.stack ? err.stack : err) + '\n');
    emit({ ok: false, error: { type: 'internal', code: 'build_error', message: String(err && err.message ? err.message : err) } }, 1);
  }
})();
