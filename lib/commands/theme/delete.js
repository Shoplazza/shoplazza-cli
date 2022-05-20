const ora = require('ora');
const chalk = require('chalk');
const Sentry = require('@sentry/node');
const inquirer = require('inquirer');
const { getThemeDetail, getThemes, deleteTheme } = require('../../openAPI/api');
const { get } = require('../../db/user');
const log = require('../../log');
const { formatThemeList } = require('../../utils');

const confirmAndDeleteTheme = async (themeName, themeId) => {
  const answers = await inquirer.prompt([
    {
      name: 'confirm',
      type: 'list',
      message: `Are you sure you want to delete ${themeName} on ${get('store_domain')}? ${chalk.yellow(
        '(Choose with ↑ ↓ ⏎)'
      )}`,
      choices: ['Yes', 'No']
    }
  ]);
  if (answers.confirm === 'Yes') {
    try {
      await deleteTheme(themeId);
      log.info(chalk.green(`✓ ${themeName} (${themeId}) theme deleted`));
    } catch (error) {
      log.error(chalk.red(`✗ Failed to delete theme`));
      Sentry.captureException(error);
    }
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
      return confirmAndDeleteTheme(data.data.name, options.theme);
    } else {
      const storeDomain = get('store_domain');
      const spinner = ora(chalk.cyan(`Fetching theme lists from ${storeDomain}`)).start();
      const { data } = await getThemes();
      spinner.stop();
      inquirer
        .prompt([
          {
            name: 'theme',
            type: 'rawlist',
            message: `Select a theme to delete ${chalk.cyan('(Choose with ↑ ↓ ⏎)')}`,
            choices: formatThemeList(data.data.themes)
          }
        ])
        .then((answers) => {
          const themeObj = data.data.themes.find((theme) => theme.id === answers.theme);
          return confirmAndDeleteTheme(themeObj.name, answers.theme);
        });
    }
  } catch (error) {
    log.error(chalk.red(`✗ Failed to delete theme`));
    Sentry.captureException(error);
  }
};
