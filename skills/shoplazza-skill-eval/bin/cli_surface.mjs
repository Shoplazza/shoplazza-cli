// cli_surface.mjs — cached CLI-surface builder for the L0 drift sentinel.
// Shells out to the shoplazza binary (`--help` / `schema`) and exposes
// existence checks for command paths, shortcuts, and flags.
// Plain Node ESM, no external deps. Node >= 16.

import { execFileSync } from 'node:child_process';

const HELP_TIMEOUT_MS = 15000;

export class CliSurface {
  /** @param {string} bin path to the shoplazza binary */
  constructor(bin = './shoplazza') {
    this.bin = bin;
    this.helpCache = new Map(); // key: path joined by ' ' -> help text | null
    this.schemaCache = new Map(); // key: 'mod.cmd' -> boolean
    this.unionCache = new Map(); // key: svc -> Set of flags
    this.bodyCache = new Map(); // key: 'mod.cmd' -> { names, freeform, resolved }
    this.modules = null; // Set of top-level module names
  }

  /** Run the binary, return stdout string or null on error. */
  _run(args) {
    try {
      return execFileSync(this.bin, args, {
        encoding: 'utf8',
        timeout: HELP_TIMEOUT_MS,
        stdio: ['ignore', 'pipe', 'pipe'],
      });
    } catch (err) {
      // Non-zero exit (e.g. unknown schema) — return stdout if any, else null.
      if (err && typeof err.stdout === 'string' && err.stdout.length) return err.stdout;
      return null;
    }
  }

  /** Cached `--help` text for a command path (array of segments). */
  help(path) {
    const key = path.join(' ');
    if (this.helpCache.has(key)) return this.helpCache.get(key);
    const out = this._run([...path, '--help']);
    this.helpCache.set(key, out);
    return out;
  }

  /** Top-level module names (e.g. discounts, products, auth, schema, api…). */
  getModules() {
    if (this.modules) return this.modules;
    const text = this.help([]) || '';
    this.modules = parseAvailableCommands(text);
    return this.modules;
  }

  /** Subcommand names listed under a command path's "Available Commands:". */
  subcommands(path) {
    const text = this.help(path);
    if (!text) return new Set();
    return parseAvailableCommands(text);
  }

  /**
   * Does the command path exist? Walks each segment against the parent's
   * "Available Commands" list (cobra prints parent help + exit 0 for unknown
   * subcommands, so exit codes are NOT trustworthy).
   * path = [svc, ...cmds], e.g. ['discounts', 'coupons', 'create'].
   */
  hasCommandPath(path) {
    if (path.length === 0) return false;
    if (!this.getModules().has(path[0])) return false;
    for (let i = 1; i < path.length; i++) {
      const parent = path.slice(0, i);
      if (!this.subcommands(parent).has(path[i])) return false;
    }
    return true;
  }

  /** Flag names (Set of '--long') accepted by an existing command path. */
  flags(path) {
    const text = this.help(path);
    if (!text) return new Set();
    return parseFlags(text);
  }

  /** Union of flags across every command under a module (depth-limited). */
  flagUnion(svc, maxDepth = 3) {
    if (this.unionCache.has(svc)) return this.unionCache.get(svc);
    const union = new Set();
    const walk = (path, depth) => {
      for (const f of this.flags(path)) union.add(f);
      if (depth >= maxDepth) return;
      for (const sub of this.subcommands(path)) walk([...path, sub], depth + 1);
    };
    if (this.getModules().has(svc)) walk([svc], 1);
    this.unionCache.set(svc, union);
    return union;
  }

  /** Does `schema <mod>.<cmd>` resolve? (exit 2 + {"ok":false} when unknown) */
  schemaRefOk(ref) {
    if (this.schemaCache.has(ref)) return this.schemaCache.get(ref);
    const out = this._run(['schema', ref, '--format', 'json']);
    const ok = !!out && !/"ok":\s*false/.test(out) && /"summary"|"module"|"commands"/.test(out);
    this.schemaCache.set(ref, ok);
    return ok;
  }

  /**
   * Field names accepted in the request (query parameters + body) of a leaf
   * command, for validating inline `--data` / `--params` JSON object keys.
   * `path` is the command-path array, e.g. ['shop','metafields-resource','create'].
   *
   * Returns { names:Set<string>, freeform:Set<string>, resolved:boolean }:
   *   names    — every enumerable field name anywhere in the request schema
   *              (bodies nest under a wrapper, so both the wrapper and its inner
   *              fields are collected: `{"application_charge":{name,price,…}}`
   *              → application_charge, name, price, …).
   *   freeform — names whose subtree is a freeform map / protobuf.Value /
   *              recursive struct (e.g. metafields `value`, analytics `filters`);
   *              keys nested UNDER these must not be checked (arbitrary shape).
   *   resolved — true only when `schema … --view request` returned a real leaf
   *              schema with at least one field. Shortcuts / `api rest` / module
   *              overviews / unknown paths → resolved:false (caller skips).
   * Cached per command path.
   */
  bodyFields(path) {
    const key = path.join('.');
    if (this.bodyCache.has(key)) return this.bodyCache.get(key);
    const out = this._run(['schema', key, '--view', 'request', '--format', 'json']);
    const res = collectBodyFields(out);
    this.bodyCache.set(key, res);
    return res;
  }
}

/** Child field list of a schema field: object `schema.fields` or array `items.schema.fields`. */
function childFields(f) {
  const sch = f && f.schema;
  if (sch && Array.isArray(sch.fields)) return sch.fields;
  const it = f && f.items;
  if (it && it.schema && Array.isArray(it.schema.fields)) return it.schema.fields;
  return null;
}

/**
 * A field whose value shape is freeform — keys under it are caller-defined and
 * MUST NOT be checked. Detected liberally (over-detecting only under-flags):
 *  - recursive struct / protobuf.Value / Any anywhere in the subtree
 *    (metafields `value` nests a cyclic google.protobuf.Value), or
 *  - a bare object/map with no enumerable sub-fields (analytics `filters`).
 */
function isFreeformField(f) {
  const blob = JSON.stringify(f);
  if (/"cycle"\s*:\s*true/.test(blob)) return true;
  if (/protobuf/i.test(blob)) return true;
  if ((f.type === 'object' || f.type === 'map') && !childFields(f)) return true;
  return false;
}

/**
 * Parse `schema … --view request --format json` stdout into the request field
 * set. Exported for reuse/testing. Never throws — bad/absent input → resolved:false.
 */
export function collectBodyFields(jsonText) {
  const empty = { names: new Set(), freeform: new Set(), resolved: false };
  if (!jsonText) return empty;
  let doc;
  try { doc = JSON.parse(jsonText); } catch { return empty; }
  if (!doc || typeof doc !== 'object' || doc.ok === false) return empty;
  const names = new Set();
  const freeform = new Set();
  const walk = (fields) => {
    if (!Array.isArray(fields)) return;
    for (const f of fields) {
      if (!f || typeof f !== 'object') continue;
      if (f.name) names.add(f.name);
      if (f.name && isFreeformField(f)) { freeform.add(f.name); continue; }
      const kids = childFields(f);
      if (kids) walk(kids);
    }
  };
  if (Array.isArray(doc.parameters)) walk(doc.parameters);
  if (doc.body && Array.isArray(doc.body.fields)) walk(doc.body.fields);
  return { names, freeform, resolved: names.size > 0 };
}

/** Parse an "Available Commands:" block into a Set of names. */
export function parseAvailableCommands(helpText) {
  const names = new Set();
  const lines = helpText.split('\n');
  let inBlock = false;
  for (const line of lines) {
    if (/^Available Commands:/.test(line)) { inBlock = true; continue; }
    if (inBlock) {
      if (/^\S/.test(line) || line.trim() === '') { if (line.trim() === '' ) { inBlock = false; continue; } inBlock = false; continue; }
      const m = line.match(/^\s{2}(\+?[a-z0-9][a-z0-9-]*)\s/);
      if (m) names.add(m[1]);
    }
  }
  return names;
}

/** Parse "Flags:" + "Global Flags:" blocks into a Set of '--long' names. */
export function parseFlags(helpText) {
  const flags = new Set();
  const lines = helpText.split('\n');
  let inBlock = false;
  for (const line of lines) {
    if (/^(Flags|Global Flags):/.test(line)) { inBlock = true; continue; }
    if (inBlock && /^\S/.test(line) && line.trim() !== '') { inBlock = false; }
    if (!inBlock) continue;
    const m = line.match(/^\s+(?:-[a-zA-Z],\s+)?(--[a-z][a-z0-9-]*)/);
    if (m) flags.add(m[1]);
  }
  // help flags always accepted
  flags.add('--help');
  return flags;
}
