const path = require('path');
const os = require('os');
const fs = require('fs-extra');
const ora = require('ora');
const chalk = require('chalk');
const inquirer = require('inquirer');
const Sentry = require('@sentry/node');
const { get } = require('../../db/user');
const { pullTheme, getThemes, getDefaultThemeDetail } = require('../../openAPI/api');
const { unzipTheme, formatThemeList } = require('../../utils');
const log = require('../../log');

const pullThemeFiles = async (theme) => {
  const storeDomain = get('store_domain');
  const spinner = ora(chalk.cyan(`Pulling theme files from ${theme} on ${storeDomain}`)).start();
  const { data } = await pullTheme(theme);
  const zipPath = path.join(path.join(os.homedir(), '/.cache/shoplazza_theme.zip'));
  const write = fs.createWriteStream(zipPath);
  return new Promise((resolve, reject) => {
    data
      .pipe(write)
      .on('finish', async () => {
        unzipTheme(zipPath, path.resolve(process.cwd()));
        spinner.stop();
        log.info(chalk.green(`✓ Theme pulled successfully`));
        resolve();
      })
      .on('error', (err) => {
        reject(err);
        log.error(chalk.red('✗ Error pulling theme ', err));
        Sentry.captureException(err);
      });
  });
};

module.exports = async (options) => {
  try {
    if (options.theme) {
      await pullThemeFiles(options.theme);
    } else {
      const storeDomain = get('store_domain');
      const spinner = ora(chalk.cyan(`Fetching theme lists from ${storeDomain}`)).start();
      const [themesRes, defaultThemeRes] = await Promise.all([getThemes(), getDefaultThemeDetail()]);
      spinner.stop();
      const answers = await inquirer.prompt([
        {
          name: 'theme',
          type: 'rawlist',
          message: `Select a theme to pull from ${chalk.cyan('(Choose with ↑ ↓ ⏎)')}`,
          choices: formatThemeList(
            [defaultThemeRes.data.data, ...themesRes.data.data.themes],
            defaultThemeRes.data.data.id
          )
        }
      ]);
      await pullThemeFiles(answers.theme);
    }
  } catch (error) {
    log.error(chalk.red(`✗ Failed to pull theme`));
    Sentry.captureException(error);
  }
};
