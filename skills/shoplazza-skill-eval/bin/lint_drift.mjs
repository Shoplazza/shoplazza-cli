#!/usr/bin/env node
// lint_drift.mjs — L0 drift sentinel (eval-system-design §6).
// Extracts every `shoplazza <svc> …` command / `+shortcut` / `--flag` token a
// SKILL.md references and asserts each exists in the real binary's
// `--help` / `schema` surface. Also asserts the mandatory template backbone.
// Exit non-zero + print `file:line → unknown token` on any miss.
//
// Usage:
//   node bin/lint_drift.mjs <SKILL.md> [more.md …]
//     [--bin ./shoplazza]        path to the binary (default ./shoplazza)
//     [--svc auto|none|<name>]   svc context for bare `+shortcut`/`--flag`
//                                spans (auto = infer from shoplazza-<svc> dir)
//     [--backbone auto|require|skip]  backbone headings check
//                                (auto = skip for shoplazza-common)
//     [--allow tok1,tok2]        extra allowlisted pseudo-tokens
//     [--json]                   machine-readable findings
//
// Checks existence/structure ONLY — never semantics (that is L1/L2's job).

import { readFileSync } from 'node:fs';
import { basename, dirname, resolve } from 'node:path';
import { CliSurface } from './cli_surface.mjs';
import { extractReferences, expandBracePaths } from './extract_commands.mjs';

// Pseudo-tokens that appear in prose examples but are not CLI facts.
const DEFAULT_ALLOW = new Set([
  '--flag', '--shortcut', // template placeholders
  '+shortcut', '+shortcuts', '+verb', // generic tier prose ("prefer +shortcut")
]);

// Cross-cutting flags owned by shoplazza-common (all verified against the
// binary). A domain skill may name them in prose without a command context
// (e.g. "`--fields` is documented in shoplazza-common"), so bare-flag union
// checks must not flag them. Flags inside a full invocation are still strict.
const COMMON_MECHANIC_FLAGS = new Set([
  '--dry-run', '--jq', '--fields', '--format', '--profile', '--params',
  '--data', '--domain', '--scope', '--store-domain', '--uat', '--check',
  '--view', '--help',
]);

// Negative mentions: a line that *documents* a nonexistent token on purpose
// ("There is no `+update`", "`--buy/--get` rejected", "unknown flag: --discount",
// or a tier-trap gotcha cell like "`customers +count` → unknown command").
// Applies to bare `+shortcut`/`--flag` mentions AND to `unknown-command` findings
// for full invocations. Runnable recipes/fences don't carry negation words, so
// they still fail strictly when they name a command that truly drifted away.
const NEGATION_RE = /\b(no|not|never|don'?t|doesn'?t|isn'?t|aren'?t|rejected|unknown|invalid|fails?|instead of|不存在|没有|不要|别)\b/i;

// A `--data`/`--params` value is only checked when it resolves to a concrete
// JSON object literal. Anything with a placeholder marker — angle-bracket
// (`<id>`), ellipsis (`…`/`...`), template (`{{`), or a shell var (`$VAR`) —
// is skipped (under-flagging is safe; false positives are not). `@file` and
// `-` (stdin) are handled separately in checkBodyKeys.
const BODY_PLACEHOLDER_RE = /<[^>]*>|…|\.\.\.|\{\{|\$[A-Za-z_{]/;

// Walk a parsed --data/--params object, collecting keys absent from the
// command's request schema field set. Descent stops at freeform subtrees
// (metafields `value`, analytics `filters`) whose keys are caller-defined.
function collectUnknownKeys(obj, bf, out) {
  if (Array.isArray(obj)) {
    for (const it of obj) collectUnknownKeys(it, bf, out);
  } else if (obj && typeof obj === 'object') {
    for (const [k, val] of Object.entries(obj)) {
      if (!bf.names.has(k)) out.push(k);
      if (bf.freeform.has(k)) continue; // freeform region: keys are arbitrary
      collectUnknownKeys(val, bf, out);
    }
  }
}

// Resolve one inline --data/--params value against the command's request
// schema. Returns the unique unknown keys, or null when not confidently
// checkable (placeholder / stdin / @file / unparseable / no leaf schema).
function checkBodyKeys(surface, path, value) {
  const v = (value || '').trim();
  if (!v || v === '-' || v[0] === '@') return null; // stdin / @file
  if (BODY_PLACEHOLDER_RE.test(v)) return null;      // placeholder / partial
  if (v[0] !== '{' && v[0] !== '[') return null;     // not a JSON literal
  let doc;
  try { doc = JSON.parse(v); } catch { return null; } // unparseable → skip
  if (!doc || typeof doc !== 'object') return null;
  const bf = surface.bodyFields(path);
  if (!bf.resolved) return null;                     // shortcut/api/module → skip
  const out = [];
  collectUnknownKeys(doc, bf, out);
  return [...new Set(out)];
}

// Backbone requirements (AUTHORING.md §2; heading text tolerated in EN or ZH).
const BACKBONE = [
  { name: 'frontmatter name+description', test: (md) => /^---\n[\s\S]*?\bname:\s*\S+[\s\S]*?\bdescription:[\s\S]*?\n---/m.test(md) },
  { name: 'CRITICAL read-shoplazza-common line', test: (md) => /CRITICAL.*shoplazza-common\/SKILL\.md/.test(md) },
  { name: '## Overview', test: (md) => /^##\s+Overview\b/m.test(md) },
  { name: '## Command map', test: (md) => /^##\s+.*(Command map|命令地图)/m.test(md) },
  { name: '## Permissions · Scope', test: (md) => /^##\s+.*(Permissions|权限)/m.test(md) },
  { name: '## Gotchas', test: (md) => /^##\s+Gotchas\b/m.test(md) },
  { name: '## References', test: (md) => /^##\s+References\b/m.test(md) },
];

function parseArgs(argv) {
  const opts = { files: [], bin: './shoplazza', svc: 'auto', backbone: 'auto', allow: new Set(DEFAULT_ALLOW), json: false };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--bin') opts.bin = argv[++i];
    else if (a === '--svc') opts.svc = argv[++i];
    else if (a === '--backbone') opts.backbone = argv[++i];
    else if (a === '--allow') argv[++i].split(',').forEach(t => opts.allow.add(t.trim()));
    else if (a === '--json') opts.json = true;
    else opts.files.push(a);
  }
  return opts;
}

function inferSvc(file, modules) {
  const dir = basename(dirname(resolve(file)));
  const m = dir.match(/^shoplazza-(.+)$/);
  if (m && modules.has(m[1])) return m[1];
  return null; // e.g. shoplazza-common — no single svc
}

function main() {
  const opts = parseArgs(process.argv.slice(2));
  if (opts.files.length === 0) {
    console.error('usage: lint_drift.mjs <SKILL.md> [--bin ./shoplazza] [--svc auto|none|<name>] [--backbone auto|require|skip] [--allow t1,t2] [--json]');
    process.exit(2);
  }
  const surface = new CliSurface(opts.bin);
  const modules = surface.getModules();
  if (modules.size === 0) {
    console.error(`error: could not read CLI surface from '${opts.bin} --help' — is the binary built?`);
    process.exit(2);
  }

  const findings = [];
  let checked = 0;

  for (const file of opts.files) {
    const md = readFileSync(file, 'utf8');
    const svc = opts.svc === 'auto' ? inferSvc(file, modules) : (opts.svc === 'none' ? null : opts.svc);
    const isCommon = /shoplazza-common/.test(resolve(file));
    const refs = extractReferences(md, { modules });

    // ---- token existence -------------------------------------------------
    for (const ref of refs) {
      if (ref.type === 'invocation') {
        for (const path of expandBracePaths(ref.path)) {
          checked++;
          if (!surface.hasCommandPath(path)) {
            // Skip when the line documents the command as nonexistent on purpose
            // (gotcha/tier-trap cells, e.g. "`customers +count` → unknown command").
            if (!NEGATION_RE.test(ref.context || '')) {
              findings.push({ file, line: ref.line, kind: 'unknown-command', token: path.join(' '), raw: ref.raw });
            }
            continue; // no flag check against a nonexistent command
          }
          for (const flag of ref.flags) {
            checked++;
            if (opts.allow.has(flag)) continue;
            if (!surface.flags(path).has(flag)) {
              findings.push({ file, line: ref.line, kind: 'unknown-flag', token: `${path.join(' ')} ${flag}`, raw: ref.raw });
            }
          }
          // inline --data / --params body-key check (skip negated gotcha cells).
          if (!NEGATION_RE.test(ref.context || '')) {
            for (const { flag, value } of ref.dataArgs || []) {
              const unknown = checkBodyKeys(surface, path, value);
              if (unknown === null) continue; // not confidently checkable
              checked++;
              for (const key of unknown) {
                findings.push({ file, line: ref.line, kind: 'unknown-body-field', token: `${path.join(' ')} ${flag} .${key}`, raw: ref.raw });
              }
            }
          }
        }
      } else if (ref.type === 'schemaRef') {
        checked++;
        if (!surface.schemaRefOk(ref.ref)) {
          findings.push({ file, line: ref.line, kind: 'unknown-schema-ref', token: `schema ${ref.ref}`, raw: ref.raw });
        }
      } else if (ref.type === 'shortcut') {
        if (!svc) continue; // no svc context (e.g. shoplazza-common) — skip
        if (opts.allow.has(ref.name)) continue;
        if (NEGATION_RE.test(ref.context || '')) continue; // negative mention
        checked++;
        if (!surface.subcommands([svc]).has(ref.name)) {
          findings.push({ file, line: ref.line, kind: 'unknown-shortcut', token: `${svc} ${ref.name}`, raw: ref.raw });
        }
      } else if (ref.type === 'flag') {
        if (!svc) continue; // context-less flag with no svc — skip
        if (opts.allow.has(ref.name) || COMMON_MECHANIC_FLAGS.has(ref.name)) continue;
        if (NEGATION_RE.test(ref.context || '')) continue; // negative mention
        checked++;
        if (!surface.flagUnion(svc).has(ref.name)) {
          findings.push({ file, line: ref.line, kind: 'unknown-flag', token: `${svc} …${ref.name}`, raw: ref.raw });
        }
      }
    }

    // ---- backbone structure ---------------------------------------------
    const doBackbone = opts.backbone === 'require' || (opts.backbone === 'auto' && !isCommon);
    if (doBackbone) {
      for (const rule of BACKBONE) {
        checked++;
        if (!rule.test(md)) {
          findings.push({ file, line: 0, kind: 'missing-backbone', token: rule.name, raw: '' });
        }
      }
    } else if (!opts.json) {
      console.error(`note: backbone check skipped for ${file} (base skill / --backbone skip)`);
    }
  }

  if (opts.json) {
    console.log(JSON.stringify({ ok: findings.length === 0, checked, findings }, null, 2));
  } else {
    for (const f of findings) {
      console.error(`${f.file}:${f.line} → ${f.kind}: ${f.token}${f.raw ? `   [${f.raw.slice(0, 80)}]` : ''}`);
    }
    console.log(`lint_drift: ${opts.files.length} file(s), ${checked} check(s), ${findings.length} finding(s) → ${findings.length === 0 ? 'PASS' : 'FAIL'}`);
  }
  process.exit(findings.length === 0 ? 0 : 1);
}

main();
