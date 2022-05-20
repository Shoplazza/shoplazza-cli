const chalk = require('chalk');
const { fork } = require('child_process');
const path = require('path');
const open = require('open');
const log = require('../log');
const { SSO_AUTH_URL, CLIENT_ID, REDIRECT_URI } = require('../config');

const openSSOPage = () => {
  open(
    `${SSO_AUTH_URL}/switch_account?lack=0&continue=${encodeURIComponent(
      `${SSO_AUTH_URL}/api/oauth/authorize?action=login&client_id=${CLIENT_ID}&redirect_uri=${REDIRECT_URI}&response_type=code`
    )}`
  );
  log.info(`${chalk.green('✓')} Initiating authentication`);
};

const getCode = () => {
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
      }
    });
    openSSOPage();
  });
};

module.exports = getCode;
