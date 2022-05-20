const ora = require('ora');
const chalk = require('chalk');
const Sentry = require('@sentry/node');
const { get } = require('../../db/user');
const { getDefaultThemeDetail, getThemes } = require('../../openAPI/api');
const log = require('../../log');

module.exports = async () => {
  try {
    const storeDomain = get('store_domain');
    const spinner = ora(chalk.cyan(`Fetching theme lists from ${storeDomain}`)).start();
    const [themesRes, defaultThemeRes] = await Promise.all([getThemes(), getDefaultThemeDetail()]);
    spinner.stop();
    log.info(`${chalk.yellow('⭑')} List of ${chalk.green(storeDomain)} themes:`);
    log.info(
      [defaultThemeRes.data.data, ...themesRes.data.data.themes].reduce(
        (acc, theme) =>
          (acc += `${theme.name} (${chalk.green(theme.id)}) ${
            theme.id === defaultThemeRes.data.data.id ? chalk.green('[live]') : chalk.yellow('[unpublished]')
          }\n`),
        ''
      )
    );
  } catch (error) {
    log.error(chalk.red(`✗ Failed to get theme list`));
    Sentry.captureException(error);
  }
};
