const ora = require('ora');
const chalk = require('chalk');
const inquirer = require('inquirer');
const Sentry = require('@sentry/node');
const { getThemeDetail, getThemes, publishTheme } = require('../../openAPI/api');
const { get } = require('../../db/user');
const log = require('../../log');
const { formatThemeList } = require('../../utils');

const confirmAndExecPublish = async (themeName, themeId) => {
  const answers = await inquirer.prompt([
    {
      name: 'confirm',
      type: 'list',
      message: `Are you sure you want to make ${themeName} the new live theme on ${get('store_domain')}? ${chalk.yellow(
        '(Choose with ↑ ↓ ⏎)'
      )}`,
      choices: ['Yes', 'No']
    }
  ]);
  if (answers.confirm === 'Yes') {
    await publishTheme(themeId);
    log.info(chalk.green(`✓ Your theme is now live at https://${get('store_domain')}`));
  }
};

module.exports = async (options) => {
  try {
    if (options.theme) {
      const { data } = await getThemeDetail(options.theme);
      if (!data) {
        log.error(chalk.red(`✗ Theme ${options.theme} does not exist`));
        return;
      }
      await confirmAndExecPublish(data.data.name, options.theme);
    } else {
      const storeDomain = get('store_domain');
      const spinner = ora(chalk.cyan(`Fetching theme lists from ${storeDomain}`)).start();
      const { data } = await getThemes();
      spinner.stop();
      const answers = await inquirer.prompt([
        {
          name: 'theme',
          type: 'rawlist',
          message: `Select a theme to publish ${chalk.cyan('(Choose with ↑ ↓ ⏎)')}`,
          choices: formatThemeList(data.data.themes)
        }
      ]);
      const themeObj = data.data.themes.find((theme) => theme.id === answers.theme);
      await confirmAndExecPublish(themeObj.name, answers.theme);
    }
  } catch (error) {
    log.error(chalk.red(`✗ Failed to publish theme`));
    Sentry.captureException(error);
  }
};
