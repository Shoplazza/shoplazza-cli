const chalk = require('chalk');
const { fork } = require('child_process');
const path = require('path');
const open = require('open');
const log = require('../log');
const { REDIRECT_URI } = require('../config');
const { getSSOAuthUrl, getClientId } = require('../utils');

const openSSOPage = (store) => {
  open(
    `${getSSOAuthUrl(store)}/switch_account?lack=0&continue=${encodeURIComponent(
      `${getSSOAuthUrl(store)}/api/oauth/authorize?action=login&client_id=${getClientId(
        store
      )}&redirect_uri=${REDIRECT_URI}&response_type=code`
    )}`
  );
  log.info(`${chalk.green('✓')} Initiating authentication`);
};

const getCode = (store) => {
  return new Promise((resolve, reject) => {
    const child = fork(path.join(__dirname, './child.js'), { timeout: 60 * 1000 });
    child.on('message', function (message) {
      if (message.code) {
        resolve(message.code);
      }
    });
    child.on('close', function (message) {
      // Timeout
      if (message === null) {
        log.error(chalk.red('✗ Timed out while waiting for response from Shoplazza'));
        reject('timeout');
      }
    });
    openSSOPage(store);
  });
};

module.exports = getCode;
