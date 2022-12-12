const chalk = require('chalk');
const log = require('../log');
const { empty } = require('./db/partner');

const logoutPartner = () => {
  try {
    empty();
    log.info('\n', chalk.green(`Logged out your partner account successfully!`), '\n');
  } catch (e) {
    log.error(chalk.red(`✗ Failed to logout your partner account`));
  }
};

module.exports = {
  logoutPartner
};
