const chalk = require('chalk');
const inquirer = require('inquirer');
const Sentry = require('@sentry/node');
const { get } = require('../db/user');
const log = require('../log');
const { postExchangeToken } = require('../auth');

const switchStore = async (store) => {
  if (!get('access_token')) {
    log.error(chalk.red(`✗ Please login again with ${chalk.cyan('shoplazza login')}`));
  } else {
    try {
      await postExchangeToken(store);
      log.info(`Switched store to ${chalk.green(store)}`);
    } catch (error) {
      log.error(chalk.red(`✗ Failed to switch store`));
      Sentry.captureException(error);
    }
  }
};

module.exports = async (options) => {
  try {
    if (options.store) {
      return switchStore(options.store);
    } else {
      const answers = await inquirer.prompt([
        {
          name: 'store',
          type: 'input',
          message: 'The store domain (Eg: developer.myshoplaza.com )'
        }
      ]);
      if (answers.store) {
        if (!/.+\.myshoplaza\.com/.test(answers.store)) {
          log.error(
            chalk.red(
              `✗ Invalid store provided ${answers.store}. Please provide the store in the following format: developer.myshoplaza.com`
            )
          );
          process.exit(-1);
        }
        return switchStore(answers.store);
      } else {
        log.error(chalk.red(`✗ Please input the store domain.`));
      }
    }
  } catch (error) {
    log.error(chalk.red(`✗ Failed to switch store`));
    Sentry.captureException(error);
  }
};
