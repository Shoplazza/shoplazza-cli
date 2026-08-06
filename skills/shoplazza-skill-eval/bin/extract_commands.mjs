// extract_commands.mjs — tokenizer shared by the L0 drift sentinel.
// Extracts `shoplazza <svc> …` invocations, `+shortcut` tokens, `--flag`
// tokens, and `schema <mod>.<cmd>` refs from a SKILL.md: fenced ```bash
// blocks, tables, and inline code spans.
// Plain Node ESM, no external deps. Node >= 16.

/**
 * Reference kinds returned:
 *  - { type:'invocation', line, path:[svc,...cmds], flags:['--x',…],
 *      dataArgs:[{flag:'--data'|'--params', value:'{…}'}], raw }
 *  - { type:'shortcut',   line, name:'+search', raw }          // bare inline `+x`
 *  - { type:'flag',       line, name:'--limit-max', raw }      // bare inline `--x`
 *  - { type:'schemaRef',  line, ref:'discounts.get', raw }
 */
export function extractReferences(markdown, { modules }) {
  const refs = [];
  const lines = markdown.split('\n');

  // Strip YAML frontmatter (routing keywords live there; not command facts).
  let start = 0;
  if (lines[0] === '---') {
    const end = lines.indexOf('---', 1);
    if (end > 0) start = end + 1;
  }

  let inFence = false;
  let cont = null; // pending backslash-continued line: { text, line }
  const flushCont = () => { if (cont) { scanCodeText(cont.text, cont.line, modules, refs, cont.text); cont = null; } };
  for (let i = start; i < lines.length; i++) {
    const line = lines[i];
    const lineNo = i + 1;
    const fence = line.match(/^\s*```(\w*)/);
    if (fence) { flushCont(); inFence = !inFence; continue; }

    if (inFence) {
      // Join `\`-continued lines into one logical invocation (attributed to the
      // first line) so a `--data` on a continuation attaches to its command.
      const stripped = stripShellComment(line);
      const piece = stripped.replace(/\\\s*$/, ' ');
      if (cont) cont.text += piece; else cont = { text: piece, line: lineNo };
      if (!/\\\s*$/.test(stripped)) flushCont();
    } else {
      flushCont();
      // inline code spans (includes table cells)
      for (const span of line.matchAll(/`([^`]+)`/g)) {
        scanCodeText(span[1], lineNo, modules, refs, negationContext(line, span.index));
      }
    }
  }
  flushCont();
  return refs;
}

/**
 * The span of text the caller's negation check should see. Gotcha rows are long
 * and negation words ("no", "never", "unknown", "rejected") cluster in the Cause
 * column, so passing the whole ROW exempted every token in it — a misspelled
 * flag in the Fix column rode out on a "never" three cells away.
 *
 * Narrow to the containing CELL, with one exception that follows the table's own
 * contract: the FIRST cell is the Symptom, which names the broken thing on
 * purpose ("Invented `+delete` shortcut") while its explanation sits in the
 * adjacent Cause cell — so it stays row-scoped. Later cells prescribe, and get
 * checked strictly. Non-table lines keep the whole line; escaped pipes (`\|`,
 * how a literal | is written inside a cell) are not separators.
 */
function negationContext(line, index) {
  if (!/^\s*\|/.test(line)) return line;
  const seps = [];
  for (let i = 0; i < line.length; i++) {
    if (line[i] === '|' && line[i - 1] !== '\\') seps.push(i);
  }
  let start = 0, cell = -1;
  for (const s of seps) {
    if (s >= index) break;
    start = s + 1;
    cell++;
  }
  if (cell <= 0) return line; // Symptom column
  const end = seps.find((s) => s >= index);
  return end === undefined ? line.slice(start) : line.slice(start, end);
}

function stripShellComment(line) {
  // Remove a trailing "# comment" that is not inside quotes (pragmatic scan).
  let inS = false, inD = false;
  for (let i = 0; i < line.length; i++) {
    const c = line[i];
    if (c === "'" && !inD) inS = !inS;
    else if (c === '"' && !inS) inD = !inD;
    else if (c === '#' && !inS && !inD) return line.slice(0, i);
  }
  return line;
}

/** Shell-ish tokenizer that keeps quoted strings as single value tokens. */
function tokenize(text) {
  const tokens = [];
  const re = /'[^']*'|"[^"]*"|\S+/g;
  for (const m of text.matchAll(re)) tokens.push(m[0]);
  return tokens;
}

const CMD_TOKEN = /^\+?[a-z][a-z0-9-]*$/;
const BRACE_GROUP = /^\{[a-z0-9,+-]+\}$/;

function scanCodeText(text, lineNo, modules, refs, context = '') {
  // unwrap $( … ) command substitutions so inner invocations are visible
  const cleaned = text.replace(/\$\(/g, ' ').replace(/\)/g, ' ');
  const tokens = tokenize(cleaned);

  // Bare inline references (span is a single token, no command context):
  if (tokens.length === 1) {
    const t = tokens[0];
    if (/^\+[a-z][a-z0-9-]*$/.test(t)) { refs.push({ type: 'shortcut', line: lineNo, name: t, raw: text, context }); return; }
    const f = normalizeFlagToken(t);
    if (f) { refs.push({ type: 'flag', line: lineNo, name: f, raw: text, context }); return; }
    return; // single bare word — too ambiguous, skip (documented limitation)
  }

  // Full invocations: a token equal to 'shoplazza' or a known module name.
  for (let i = 0; i < tokens.length; i++) {
    let j = i;
    if (tokens[j] === 'shoplazza') j++;
    const svc = tokens[j];
    // explicit `shoplazza <badmodule>` — flag the unknown module itself
    if (tokens[j - 1] === 'shoplazza' && svc && !modules.has(svc) && /^[a-z][a-z0-9-]*$/.test(svc)) {
      refs.push({ type: 'invocation', line: lineNo, path: [svc], flags: [], raw: text, context });
      return;
    }
    if (!svc || !modules.has(svc)) {
      // lone flags inside a multi-token non-invocation span, e.g. `--jq '.x'`
      if (i === 0 && !tokens.includes('shoplazza') && !tokens.some(t => modules.has(t))) {
        for (const t of tokens) {
          const f = normalizeFlagToken(t);
          if (f) refs.push({ type: 'flag', line: lineNo, name: f, raw: text, context });
        }
        // bare `+shortcut` leading a non-invocation span, e.g. `+bxgy-code --buy`
        if (/^\+[a-z][a-z0-9-]*$/.test(tokens[0])) {
          refs.push({ type: 'shortcut', line: lineNo, name: tokens[0], raw: text, context });
        }
        return;
      }
      continue;
    }
    if (tokens[j - 1] !== 'shoplazza' && j > 0 && tokens[j - 1] !== undefined && !/[;&|(]$|^\S*=$/.test(tokens[j - 1]) && !isStopToken(tokens[j - 1])) {
      // svc word appears mid-sentence inside a code span (e.g. a jq path) — require
      // it to start the span, follow 'shoplazza', or follow a shell separator.
      if (j !== 0) continue;
    }

    // schema special-case: `schema <mod>.<cmd>` or `schema <mod>`
    if (svc === 'schema') {
      const arg = tokens[j + 1];
      const flags = collectFlags(tokens.slice(j + 1));
      if (arg && /^[a-z][a-z0-9-]*\.[a-z][a-z0-9-]*$/.test(arg)) {
        refs.push({ type: 'schemaRef', line: lineNo, ref: arg, raw: text, context });
      } else if (arg && /^[a-z][a-z0-9-]*$/.test(arg) && modules.has(arg)) {
        refs.push({ type: 'schemaRef', line: lineNo, ref: arg, raw: text, context });
      }
      refs.push({ type: 'invocation', line: lineNo, path: ['schema'], flags, raw: text, context });
      return;
    }

    // consume command path
    const path = [svc];
    let k = j + 1;
    let placeholder = false;
    while (k < tokens.length) {
      const t = tokens[k];
      if (/<|{{/.test(t) && !BRACE_GROUP.test(t)) { placeholder = /^[<{]/.test(t) ? true : placeholder; break; }
      if (BRACE_GROUP.test(t)) { path.push(t); k++; break; }
      if (!CMD_TOKEN.test(t)) break;
      // api rest: stop after 'rest' (METHOD/PATH follow)
      path.push(t); k++;
      if (svc === 'api' && t === 'rest') break;
    }
    if (placeholder) return; // prose example like `shoplazza <svc> <cmd> --help`
    const rest = tokens.slice(k);
    const flags = collectFlags(rest);
    const dataArgs = collectDataArgs(rest);
    refs.push({ type: 'invocation', line: lineNo, path, flags, dataArgs, raw: text, context });
    return; // one invocation per span/line is enough for linting
  }
}

function isStopToken(t) {
  return t === undefined || /^[;&|>]+$/.test(t);
}

/** Normalize a token to a '--long' flag name, or null. */
function normalizeFlagToken(tok) {
  let t = tok;
  // strip optional-usage brackets: [--limit-max → --limit-max
  t = t.replace(/^[[(]+/, '').replace(/[\])]+$/, '');
  if (!t.startsWith('--')) return null;
  // strip =value, trailing punctuation, and '/-alternation' tails:
  // `--limit-user-variant/-product/-all` → --limit-user-variant
  t = t.split('=')[0].split('/')[0];
  t = t.replace(/[.,;:…]+$/, '');
  if (!/^--[a-z][a-z0-9-]*$/.test(t)) return null;
  return t;
}

/** Collect flag names from the value/flag remainder of an invocation. */
function collectFlags(tokens) {
  const flags = [];
  for (const t of tokens) {
    if (/^[>|]|^&&$/.test(t)) break; // shell redirect / pipe: stop
    const f = normalizeFlagToken(t);
    if (f) flags.push(f);
  }
  return [...new Set(flags)];
}

/** Strip one matching layer of surrounding single/double quotes. */
function unquote(s) {
  if (s.length >= 2 && ((s[0] === "'" && s[s.length - 1] === "'") || (s[0] === '"' && s[s.length - 1] === '"'))) {
    return s.slice(1, -1);
  }
  return s;
}

/**
 * Pair `--data` / `--params` with their value token (both `--data '{…}'` and
 * `--data='{…}'` forms), returning [{flag, value}] with surrounding quotes
 * stripped. Values are raw strings for the body-key check to guard + parse.
 */
function collectDataArgs(tokens) {
  const out = [];
  for (let i = 0; i < tokens.length; i++) {
    const t = tokens[i];
    if (/^[>|]|^&&$/.test(t)) break; // shell redirect / pipe: stop
    const eq = t.match(/^(--data|--params)=(.*)$/s);
    if (eq) { out.push({ flag: eq[1], value: unquote(eq[2]) }); continue; }
    if (t === '--data' || t === '--params') {
      const v = tokens[i + 1];
      if (v !== undefined) { out.push({ flag: t, value: unquote(v) }); i++; }
    }
  }
  return out;
}

/** Expand `{a,b,c}` path segments into concrete paths. */
export function expandBracePaths(path) {
  let paths = [[]];
  for (const seg of path) {
    if (BRACE_GROUP.test(seg)) {
      const opts = seg.slice(1, -1).split(',');
      paths = paths.flatMap(p => opts.map(o => [...p, o]));
    } else {
      paths = paths.map(p => [...p, seg]);
    }
  }
  return paths;
}
