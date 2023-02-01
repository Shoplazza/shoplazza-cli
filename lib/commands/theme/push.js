const ora = require('ora');
const chalk = require('chalk');
const inquirer = require('inquirer');
const Sentry = require('@sentry/node');
const fs = require('fs-extra');
const { get } = require('../../db/user');
const { getThemeDetail, getThemes, getDefaultThemeDetail, pushTheme, getPushTask } = require('../../openAPI/api');
const { sleep, formatThemeList } = require('../../utils');
const { checkAndZipThemes } = require('./package');
const log = require('../../log');

const checkPushTask = (taskId) => {
  let maxCheckCount = 3 * 60;
  let state = -1;
  return new Promise(async (resolve, reject) => {
    while (maxCheckCount--) {
      const {
        data: {
          task: { status, info, message }
        }
      } = await getPushTask(taskId);
      status == 0 && (await sleep(3 * 1000));
      if (status == 1 || status == 2) {
        state = status;
        state == 1 && resolve(info);
        state == 2 && reject(`Status Detection failure (2), Error message: ${message}`);
        break;
      }
    }
    state == -1 && reject('Retry timeout');
  }).catch((error) => {
    log.error(chalk.red(`\n✗ Failed to push theme`, error));
    Sentry.captureException(error);
    process.exit(-1);
  });
};

const pushThemeFiles = async (theme) => {
  let merchant_theme_id = '';
  let name = '';
  if (theme) {
    const { data } = await getThemeDetail(theme);
    if (!data) {
      log.error(chalk.red(`✗ Theme ${theme} does not exist`));
      process.exit(-1);
    }
    name = data.data.name;
    merchant_theme_id = data.data.merchant_theme_id;
  }

  const { theme_name, theme_version = '', zipPath } = checkAndZipThemes();
  const {
    data: {
      task: {
        task: { id }
      }
    }
  } = await pushTheme({
    name: name || theme_name,
    version: theme_version,
    merchant_theme_id,
    theme_id: theme || '',
    zipPath
  });
  fs.rmSync(zipPath);
  return checkPushTask(id);
};

const execPushTheme = async (theme) => {
  const storeDomain = get('store_domain');
  const spinner = ora(chalk.cyan(`Pushing theme files on ${storeDomain}`)).start();
  const { name } = JSON.parse(await pushThemeFiles(theme));
  spinner.stop();
  log.info(chalk.green(`✓ The ${name} theme pushed successfully`));
};

const pushCommand = async (options) => {
  try {
    if (options.theme) {
      await execPushTheme(options.theme);
    } else {
      const storeDomain = get('store_domain');
      const spinner = ora(chalk.cyan(`Fetching theme lists from ${storeDomain}`)).start();
      const [themesRes, defaultThemeRes] = await Promise.all([getThemes(), getDefaultThemeDetail()]);
      spinner.stop();
      const answers = await inquirer.prompt([
        {
          name: 'theme',
          type: 'rawlist',
          message: `Select a theme to push ${chalk.cyan('(Choose with ↑ ↓ ⏎)')}`,
          choices: [
            { name: ' Create a new unpublished theme', value: '' },
            ...formatThemeList([defaultThemeRes.data.data, ...themesRes.data.data.themes], defaultThemeRes.data.data.id)
          ]
        }
      ]);
      await execPushTheme(answers.theme);
    }
  } catch (error) {
    log.error(chalk.red(`✗ Failed to push theme`));
    Sentry.captureException(error);
  }
};

exports.pushCommand = pushCommand;
exports.pushThemeFiles = pushThemeFiles;
