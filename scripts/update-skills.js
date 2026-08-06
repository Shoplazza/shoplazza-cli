#!/usr/bin/env node
/**
 * Refresh globally installed shoplazza Agent Skills after a CLI install/update.
 *
 * Opt-in by prior action, never by surprise: this only runs when the user has
 * already installed shoplazza-* skills globally (~/.agents/skills/). It then
 * re-pulls exactly those skills from the CLI repo so skill docs track the CLI
 * version just installed. Detection is by directory (the skills lock file does
 * not reliably record every install).
 *
 * Never fatal: any failure leaves the CLI install successful and prints the
 * manual command instead.
 *
 * Environment overrides:
 *   SHOPLAZZA_CLI_SKIP_SKILLS   — "1" to skip entirely
 *   SHOPLAZZA_CLI_SKILLS_SOURCE — alternate `skills add` source (default:
 *                                 Shoplazza/shoplazza-cli; a local path works)
 */

'use strict';

const { spawnSync } = require('child_process');
const fs = require('fs');
const os = require('os');
const path = require('path');

const SOURCE = process.env.SHOPLAZZA_CLI_SKILLS_SOURCE || 'Shoplazza/shoplazza-cli';

function installedSkillNames() {
  const dir = path.join(os.homedir(), '.agents', 'skills');
  let entries;
  try {
    entries = fs.readdirSync(dir);
  } catch {
    return [];
  }
  return entries.filter((name) =>
    name.startsWith('shoplazza-') &&
    fs.existsSync(path.join(dir, name, 'SKILL.md')));
}

function updateSkills() {
  if (process.env.SHOPLAZZA_CLI_SKIP_SKILLS === '1') return;
  if (process.env.CI) return; // no surprise network calls in CI

  const names = installedSkillNames();
  if (names.length === 0) return; // user never installed the skills — stay out

  console.log(`Updating ${names.length} installed shoplazza skill(s) …`);
  const args = ['-y', 'skills', 'add', SOURCE, '-g', '-y', '-s', ...names];
  const res = spawnSync('npx', args, {
    stdio: ['ignore', 'inherit', 'inherit'],
    timeout: 120000,
    shell: process.platform === 'win32',
  });
  if (res.status !== 0) {
    console.warn('Skill update did not complete — the CLI itself installed fine.');
    console.warn(`Update manually: npx skills add ${SOURCE} -g -y -s ${names.join(' ')}`);
  }
}

if (require.main === module) {
  updateSkills();
}

module.exports = { updateSkills, installedSkillNames };
