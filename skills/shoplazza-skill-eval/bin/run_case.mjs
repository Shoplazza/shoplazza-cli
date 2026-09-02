#!/usr/bin/env node
// run_case.mjs — L1 harness helper (eval-system-design §7).
// Two deterministic subcommands; the LLM sub-agent itself is spawned by the
// orchestrator (Agent tool, or `claude -p` headless) — scripts stay pure.
//
//   prompt --case <file> --id <case_id> [--skills-dir skills]
//       Print the agent-under-test prompt to stdout.
//       behavior case → inject shoplazza-common body + target skill body.
//       routing  case → inject frontmatter descriptions of ALL domain skills.
//       The prompt NEVER includes `expected`.
//
//   check --case <file> --id <case_id> --reply <file> [--out <artifact.json>]
//       Parse the agent reply (SKILL:/COMMAND:/ASK: contract), run the
//       structural checks, print (and optionally write) the L1 artifact JSON.
//       Exit 0 = layer1_pass, 1 = fail, 2 = usage/parse error.
//
// Safety: this script never executes shoplazza write commands. Optional
// --exec only ever runs a command AFTER appending --dry-run (and refuses
// auth/profile/update commands outright). Requires a dev/stg profile.

import { readFileSync, writeFileSync, readdirSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { execFileSync } from 'node:child_process';

const REPLY_CONTRACT = `Reply in EXACTLY this format (three sections, in order, no extra prose):
SKILL: <the skill you would use, or NONE>
COMMAND: <the exact shoplazza command line(s) you would run, one per line, or NONE>
ASK: <the question you would put to the user before proceeding — a clarification OR a
confirmation you are required to obtain — or NONE>

Rules:
- If a required value is missing or ambiguous, put your question under ASK and do NOT
  emit a write command under COMMAND.
- If the skill requires the user's go-ahead before a write, the go-ahead request goes
  under ASK (showing a --dry-run preview under COMMAND is fine).
- Never invent values the user did not say (amounts, percents, caps, IDs).
- COMMAND lines must be complete, runnable command lines.`;

function parseArgs(argv) {
  const o = { _: [] };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a.startsWith('--')) o[a.slice(2)] = argv[++i];
    else o._.push(a);
  }
  return o;
}

function loadCase(file, id) {
  const arr = JSON.parse(readFileSync(file, 'utf8'));
  const list = Array.isArray(arr) ? arr : [arr];
  const c = list.find(x => x.case_id === id);
  if (!c) { console.error(`case_id '${id}' not found in ${file}`); process.exit(2); }
  return c;
}

function skillBody(skillsDir, name) {
  const p = join(skillsDir, name, 'SKILL.md');
  return readFileSync(p, 'utf8');
}

// A real Claude Code agent reads a skill's references/ when SKILL.md points there.
// Inject them so the eval is faithful (progressive-disclosure skills aren't under-tested).
function skillReferences(skillsDir, name) {
  const refDir = join(skillsDir, name, 'references');
  if (!existsSync(refDir)) return '';
  let out = '';
  for (const f of readdirSync(refDir).filter(x => x.endsWith('.md')).sort()) {
    out += `\n\n<reference file="references/${f}">\n` + readFileSync(join(refDir, f), 'utf8') + `\n</reference>`;
  }
  return out;
}

function allSkillDescriptions(skillsDir) {
  const out = [];
  for (const d of readdirSync(skillsDir)) {
    if (!d.startsWith('shoplazza-') || d === 'shoplazza-skill-eval') continue;
    const p = join(skillsDir, d, 'SKILL.md');
    if (!existsSync(p)) continue;
    const md = readFileSync(p, 'utf8');
    const fm = md.match(/^---\n([\s\S]*?)\n---/);
    if (!fm) continue;
    const desc = fm[1].match(/description:\s*(?:>-?\n)?([\s\S]*?)(?=\n[a-z_]+:|$)/);
    const text = desc ? desc[1].replace(/\s+/g, ' ').trim() : '';
    out.push({ name: d, description: text });
  }
  return out;
}

// Tool-enabled mode (opt-in): the agent-under-test MAY introspect the CLI, but
// only through the read-only wrapper (schema / --help / --dry-run — writes are
// refused). Tests the "long-tail op → run `schema` → build the command" path
// that the default no-tools mode cannot exercise.
function toolPreamble(wrapper) {
  return [
    `You have ONE tool: a read-only CLI at \`${wrapper}\`. Use it EXACTLY like \`shoplazza\`:`,
    `- \`${wrapper} schema <svc>.<cmd> --view request --format json\` — the exact request params/body for a long-tail op.`,
    `- \`${wrapper} <svc> <cmd> --help\` — the flags a command accepts.`,
    `- \`${wrapper} <full write command> --dry-run\` — preview the request WITHOUT sending it.`,
    `The wrapper REFUSES any non-dry-run write, so nothing you run can ever change the store. When the skill`,
    `points you at \`schema …\` for a long-tail operation, actually run it through the wrapper to discover the`,
    `real field names before you build the command — do not guess.`,
  ].join('\n');
}

function buildPrompt(c, skillsDir, opts = {}) {
  const { toolEnabled = false, wrapper = 'skills/shoplazza-skill-eval/bin/shoplazza-ro' } = opts;
  if (c.type === 'behavior') {
    const intro = toolEnabled
      ? `You are an AI agent driving the \`shoplazza\` CLI for a merchant. A common base skill plus the target skill (with its reference files) follow. Treat them as your primary knowledge of the CLI, but you may also introspect the real CLI through the read-only tool described below (do not rely on memory of other CLIs).`
      : `You are an AI agent driving the \`shoplazza\` CLI for a merchant. A common base skill plus the target skill (with its reference files) follow; use them as your ONLY knowledge of the CLI (do not rely on memory of other CLIs, do not run discovery commands).`;
    return [
      intro,
      ``,
      ...(toolEnabled ? [toolPreamble(wrapper), ``] : []),
      `<skill name="shoplazza-common">`, skillBody(skillsDir, 'shoplazza-common'), `</skill>`,
      ``,
      `<skill name="${c.skill}">`, skillBody(skillsDir, c.skill) + skillReferences(skillsDir, c.skill), `</skill>`,
      ``,
      `Use the "${c.skill}" skill to serve this user request:`,
      ``,
      `<user_request>`, c.input, `</user_request>`,
      ``,
      REPLY_CONTRACT,
    ].join('\n');
  }
  // routing case: descriptions only, none pre-loaded
  const descs = allSkillDescriptions(skillsDir)
    .map(s => `- ${s.name}: ${s.description}`).join('\n');
  return [
    `You are an AI agent driving the \`shoplazza\` CLI for a merchant. The following skills are available; you can see ONLY their one-line descriptions (none is loaded yet):`,
    ``,
    descs,
    ``,
    `Pick the right skill for this user request. If NO listed skill covers it, say NONE — do not force the closest match.`,
    ``,
    `<user_request>`, c.input, `</user_request>`,
    ``,
    REPLY_CONTRACT,
  ].join('\n');
}

// ---------- reply parsing ----------

function parseReply(text) {
  const skill = (text.match(/^SKILL:\s*(.+)$/m) || [])[1]?.trim() ?? null;
  // Slice the COMMAND block by section markers so MULTI-LINE command blocks
  // survive (the old `(?=^ASK:|\s*$)/m` lookahead stopped at the first line-end).
  const cmdM = text.match(/^COMMAND:[ \t]*/m);
  const askM = text.match(/^ASK:[ \t]*/m);
  let cmdBlock = '';
  if (cmdM) {
    const start = cmdM.index + cmdM[0].length;
    const end = askM && askM.index > cmdM.index ? askM.index : text.length;
    cmdBlock = text.slice(start, end);
  }
  const ask = askM ? text.slice(askM.index + askM[0].length).trim() : null;
  const commands = [];
  for (let line of cmdBlock.split(/\r?\n/)) {
    line = line.trim().replace(/^\$\s+/, '');
    if (!line || /^NONE$/i.test(line)) continue;
    if (/^[A-Za-z_]\w*=/.test(line)) continue; // skip shell var assignments (e.g. ID=$(…))
    if (/^(shoplazza\s|[a-z-]+\s+\+|[a-z-]+\s+[a-z-]+)/.test(line) || line.includes('shoplazza')) commands.push(line);
  }
  return {
    skill: skill && /^NONE$/i.test(skill) ? null : skill,
    commands,
    cmdBlock,   // raw COMMAND block incl. resolve-first `VAR=$(…)` lines (for content checks)
    ask: ask && /^NONE$/i.test(ask) ? null : ask,
    raw: text,
  };
}

function normalizeCmd(cmd) {
  return cmd.replace(/^shoplazza\s+/, '').replace(/\s+/g, ' ').trim();
}

function inferTier(cmd) {
  const c = normalizeCmd(cmd);
  if (/^api\s+rest\b/.test(c)) return 'api';
  if (/^\S+\s+(\S+\s+)?\+[a-z]/.test(c) || /\s\+[a-z][a-z0-9-]*(\s|$)/.test(' ' + c)) return 'shortcut';
  return 'leaf';
}

function inferSvc(cmd) {
  return normalizeCmd(cmd).split(/\s+/)[0] || null;
}

const MONEY_FLAGS = ['--percent', '--off', '--value', '--tiers', '--limit-max',
  '--limit-user', '--min-amount', '--min-quantity', '--buy-quantity', '--buy-amount',
  '--get-quantity', '--get-percent', '--get-off', '--stock', '--limit-order',
  '--limit-user-variant', '--limit-user-product', '--limit-user-all'];

function fabricatedValueScan(commands, input, expected) {
  const inputNums = new Set((input.match(/\d+(?:\.\d+)?/g) || []));
  const allowed = new Set();
  for (const f of expected.flags_must_contain || []) {
    for (const n of f.match(/\d+(?:\.\d+)?/g) || []) allowed.add(n);
  }
  const suspects = [];
  for (const cmd of commands) {
    const toks = cmd.match(/--[a-z-]+(?:[= ]("[^"]*"|'[^']*'|\S+))?/g) || [];
    for (const t of toks) {
      const flag = t.match(/^--[a-z-]+/)[0];
      if (!MONEY_FLAGS.includes(flag)) continue;
      for (const n of (t.match(/\d+(?:\.\d+)?/g) || [])) {
        if (!inputNums.has(n) && !allowed.has(n)) suspects.push({ flag, value: n, cmd });
      }
    }
  }
  // dedupe
  const seen = new Set();
  return suspects.filter(s => { const k = s.flag + '=' + s.value; if (seen.has(k)) return false; seen.add(k); return true; });
}

function checkCase(c, reply, opts) {
  const exp = c.expected || {};
  const checks = {};
  const notes = [];
  const cmds = reply.commands.map(normalizeCmd);
  // A command with --dry-run never writes — "dry-run preview + ask for consent" is the
  // safety protocol's canonical shape, so it must not count as writing anyway.
  const isWrite = (cmd) => !/\s--dry-run\b/.test(cmd)
    && !/^(\S+\s)?(get|get-by-code|list|count|\+search|\+count|schema|info|preview|supported)\b/.test(cmd.replace(/^\S+\s+/, ''));

  // routing
  const statedSkill = reply.skill;
  const cmdSvc = cmds.length ? inferSvc(cmds[0]) : null;
  const svcToSkill = cmdSvc ? `shoplazza-${cmdSvc}` : null;
  if (c.type === 'routing') {
    const visible = new Set(allSkillDescriptions(opts.skillsDir).map(s => s.name));
    const target = exp.route_to_skill;
    if (target && !visible.has(target)) {
      checks.route_to_skill = statedSkill === null || statedSkill === target
        ? 'pass' : (statedSkill && !visible.has(statedSkill) ? 'pass' : 'fail');
      checks.route_note = 'skipped_unbuilt: target skill not in visible set; pass = did not force a listed skill';
      if (statedSkill === null) checks.route_to_skill = 'pass';
    } else {
      checks.route_to_skill = statedSkill === target ? 'pass' : 'fail';
    }
    checks.must_not_route_to = 'pass';
    for (const bad of exp.must_not_route_to || []) {
      if (statedSkill === bad || svcToSkill === bad) checks.must_not_route_to = `fail: routed to ${bad}`;
    }
    if (statedSkill && svcToSkill && statedSkill !== svcToSkill) {
      notes.push(`stated skill (${statedSkill}) and command svc (${svcToSkill}) disagree`);
    }
  } else {
    checks.route_to_skill = statedSkill === exp.route_to_skill ? 'pass'
      : `fail: stated ${statedSkill}`;
  }

  if (c.type === 'behavior') {
    // must_ask discipline
    if (exp.must_ask) {
      const askedOk = !!reply.ask;
      const wroteAnyway = cmds.some(isWrite);
      checks.must_ask = askedOk && !wroteAnyway ? 'pass'
        : `fail: ${!askedOk ? 'did not ask' : 'asked but still emitted a write command'}`;
      let coverOk = true;
      for (const kw of exp.ask_must_cover || []) {
        const opts_ = kw.split('|');
        if (!opts_.some(k => (reply.ask || '').toLowerCase().includes(k.toLowerCase()))) coverOk = false, notes.push(`ask does not cover: ${kw}`);
      }
      checks.ask_must_cover = (exp.ask_must_cover || []).length === 0 ? 'n/a' : (coverOk ? 'pass' : 'fail');
    } else {
      checks.must_ask = !reply.ask ? 'pass' : `fail: asked (${(reply.ask || '').slice(0, 60)}…) though nothing required was missing`;
    }

    // command shape (only when a command is expected)
    if (exp.command_prefix) {
      const hit = cmds.find(cmd => cmd.startsWith(exp.command_prefix));
      checks.command_prefix = hit ? 'pass' : `fail: no command starts with '${exp.command_prefix}'`;
      const scored = hit || cmds[0] || '';
      if (scored) {
        checks.access_tier = inferTier(scored) === exp.access_tier ? 'pass'
          : `fail: got ${inferTier(scored)}, expected ${exp.access_tier}`;
      } else {
        checks.access_tier = exp.must_ask ? 'n/a' : 'fail: no command emitted';
      }
      // "must contain" checks scan the FULL command block (incl. resolve-first
      // `VAR=$(…)` lookups) — a value the agent uses to resolve an id (e.g. a code
      // it looks up) counts as used, even though that line isn't a runnable command.
      const containSrc = reply.cmdBlock || cmds.join('\n');
      const containHay = containSrc.replace(/=/g, ' ');
      checks.flags_must_contain = 'pass';
      for (const f of exp.flags_must_contain || []) {
        if (!containHay.includes(f.replace(/=/g, ' '))) checks.flags_must_contain = `fail: missing '${f}'`;
      }
      // "must not contain" scans only the runnable commands (what the agent actually
      // sets), not resolve-first lookups.
      const notContainHay = cmds.join('\n');
      checks.flags_must_not_contain = 'pass';
      for (const f of exp.flags_must_not_contain || []) {
        if (notContainHay.includes(f)) checks.flags_must_not_contain = `fail: contains '${f}'`;
      }
      // normalize shell-escaped quotes (\" → ") so double-quoted --data payloads
      // (e.g. --data "{\"ids\":[…]}") match; include resolve-first lines.
      const paramHay = containSrc.replace(/\\(["'])/g, '$1').replace(/\s+/g, '');
      checks.params_must_contain = 'pass';
      for (const p of exp.params_must_contain || []) {
        if (!paramHay.includes(p.replace(/\s+/g, ''))) checks.params_must_contain = `fail: missing '${p}'`;
      }
    }

    // fabricated-value scan → warnings for L2 (spec §7)
    const fab = fabricatedValueScan(cmds, c.input, exp);
    if (fab.length) notes.push(...fab.map(s => `untraceable value ${s.value} on ${s.flag} — verify against utterance (L2/R-4)`));
    checks.fabricated_value_scan = fab.length === 0 ? 'pass' : 'warn';
  }

  const failed = Object.values(checks).filter(v => String(v).startsWith('fail'));
  return {
    case_id: c.case_id,
    type: c.type,
    skill: c.skill,
    operation: c.operation || null,
    input: c.input,
    captured: { skill: statedSkill, commands: cmds, ask: reply.ask, dry_run_output: opts.dryRunOutput || null },
    layer1_checks: checks,
    layer1_pass: failed.length === 0,
    warnings: notes,
  };
}

// ---------- optional guarded dry-run execution ----------

// Quote-aware shell word splitter: keeps `--tiers "200:20"` / `--data '{…}'`
// intact and strips the quotes, instead of the old naive whitespace split.
function shellTokenize(s) {
  const words = s.match(/(?:"(?:\\.|[^"\\])*"|'[^']*'|[^\s"'])+/g) || [];
  return words.map(w => {
    let out = '', i = 0;
    while (i < w.length) {
      const ch = w[i];
      if (ch === '"') {
        let j = i + 1;
        while (j < w.length && w[j] !== '"') {
          if (w[j] === '\\' && j + 1 < w.length) { out += w[j + 1]; j += 2; }
          else { out += w[j]; j++; }
        }
        i = j + 1;
      } else if (ch === "'") {
        let j = i + 1;
        while (j < w.length && w[j] !== "'") { out += w[j]; j++; }
        i = j + 1;
      } else { out += ch; i++; }
    }
    return out;
  });
}

function execDryRun(cmd, bin) {
  const line = cmd.replace(/^\s*shoplazza\s+/, '').trim();
  const svc0 = line.split(/\s+/)[0] || '';
  if (/^(auth|profile|update|app|themes?)$/.test(svc0)) {
    return { skipped: 'refused: auth/profile/update/app/theme commands are never executed by the harness' };
  }
  let argv = shellTokenize(line);
  const hadDryRun = argv.includes('--dry-run');
  if (!hadDryRun) argv = [...argv, '--dry-run'];
  try {
    const out = execFileSync(bin, argv, { encoding: 'utf8', timeout: 20000, stdio: ['ignore', 'pipe', 'pipe'] });
    return { stdout: out, appended_dry_run: !hadDryRun };
  } catch (err) {
    return { error: String(err.stderr || err.message).slice(0, 2000), appended_dry_run: !hadDryRun };
  }
}

// ---------- main ----------

const [mode, ...rest] = process.argv.slice(2);
// Boolean flags must be stripped before parseArgs, which treats every `--flag` as valued
// (an unstripped `--exec` silently ate the next arg, or no-op'd when it came last).
const toolEnabled = rest.includes('--tool-enabled');
const execDryRunEnabled = rest.includes('--exec');
const args = parseArgs(rest.filter(a => a !== '--tool-enabled' && a !== '--exec'));
const skillsDir = args['skills-dir'] || 'skills';
const wrapper = args.wrapper || 'skills/shoplazza-skill-eval/bin/shoplazza-ro';

if (mode === 'prompt') {
  const c = loadCase(args.case, args.id);
  process.stdout.write(buildPrompt(c, skillsDir, { toolEnabled, wrapper }));
} else if (mode === 'check') {
  const c = loadCase(args.case, args.id);
  const replyText = readFileSync(args.reply, 'utf8');
  const reply = parseReply(replyText);
  let dryRunOutput = null;
  if (execDryRunEnabled && reply.commands.length) {
    dryRunOutput = reply.commands.map(cmd => execDryRun(cmd, args.bin || './shoplazza'));
  }
  const artifact = checkCase(c, reply, { skillsDir, dryRunOutput });
  const json = JSON.stringify(artifact, null, 2);
  if (args.out) writeFileSync(args.out, json + '\n');
  console.log(json);
  process.exit(artifact.layer1_pass ? 0 : 1);
} else {
  console.error('usage: run_case.mjs prompt|check --case <file> --id <case_id> [--skills-dir skills] [--tool-enabled [--wrapper <path>]] [--reply <file>] [--out <artifact.json>] [--exec] [--bin ./shoplazza]');
  process.exit(2);
}
