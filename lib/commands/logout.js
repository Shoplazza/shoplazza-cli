const Sentry = require('@sentry/node');
const chalk = require('chalk');
const { empty: emptyUser } = require('../db/user');
const { logoutPartner } = require('../app/logout');
const log = require('../log');

const logoutStore = () => {
  try {
    emptyUser();
    log.info('Successfully logged out of your store account');
  } catch (error) {
    log.error(chalk.red(`✗ Failed to logout your store account`));
    Sentry.captureException(error);
  }
};

module.exports = (options) => {
  if (options.partner) {
    logoutPartner();
  } else {
    logoutStore();
  }
};
