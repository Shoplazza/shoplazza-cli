const Sentry = require('@sentry/node');
const chalk = require('chalk');
const { empty } = require('../db/user');
const log = require('../log');

module.exports = () => {
  try {
    empty();
    log.info('Successfully logged out of your account');
  } catch (error) {
    log.error(chalk.red(`✗ Failed to logout`));
    Sentry.captureException(error);
  }
};
