const chalk = require('chalk');
const ora = require('ora');
const Sentry = require('@sentry/node');
const log = require('../../log');
const { getShopDetail } = require('../../openAPI/api');
const { pushThemeFiles } = require('./push');

module.exports = async () => {
  try {
    const { data } = await getShopDetail();
    const spinner = ora(chalk.cyan(`Pushing theme files on ${data.shop.domain}`)).start();
    const { name, theme_id } = JSON.parse(await pushThemeFiles());
    spinner.stop();
    log.info(chalk.green(`✓ The ${name} Theme pushed successfully`));
    log.info(`Share your theme preview:\n${chalk.cyan(`https://${data.shop.domain}?preview_theme_id=${theme_id}`)}`);
  } catch (error) {
    log.error(chalk.red(`✗ Failed to share theme`));
    Sentry.captureException(error);
  }
};
