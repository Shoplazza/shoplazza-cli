const chalk = require('chalk');
const Sentry = require('@sentry/node');
const { getShopDetail } = require('../openAPI/api');
const log = require('../log');

module.exports = async () => {
  try {
    const { data } = await getShopDetail();
    log.info(`You're currently logged into ${chalk.green(data.shop.domain)}`);
  } catch (error) {
    log.error(chalk.red(`✗ Failed to get store`));
    Sentry.captureException(error);
  }
};
