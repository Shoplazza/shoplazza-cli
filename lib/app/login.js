const chalk = require('chalk');
const ora = require('ora');
const open = require('open');
const axios = require('axios');
const { fork } = require('child_process');
const path = require('path');

const { set, PARTNER_KEYS, get, empty } = require('./db/partner');
const { getClientId } = require('../utils');
const { PARNTER_URL, LOGIN_BASE_URL, CLIENT_ID } = require('./constants');
const { REDIRECT_URI } = require('../config');
const log = require('../log');
const { choosePartner } = require('./inquirers/choose-partner');
const { chooseApp } = require('./inquirers/choose-app');

const isLogin = () => {
  const values = get();
  const time = new Date().getTime();

  if (!values || !values[PARTNER_KEYS.SESSION_ID]) {
    return false;
  }

  if (Number(time) - Number(values[PARTNER_KEYS.LOGIN_TIMESTAMP]) >= Number(values[PARTNER_KEYS.LOGIN_TIMESTAMP])) {
    return false;
  }

  return true;
};

const stop = async (time = 1000) => {
  return new Promise((resolve) => {
    setTimeout(() => {
      resolve(true);
    }, time);
  });
};

const openLoginPage = () => {
  open(
    `${LOGIN_BASE_URL}/switch_account?lack=0&continue=${encodeURIComponent(
      `${LOGIN_BASE_URL}/api/oauth/authorize?action=login&client_id=${CLIENT_ID}&redirect_uri=${REDIRECT_URI}&response_type=code`
    )}`
  );
};

const getCode = () => {
  return new Promise((resolve, reject) => {
    const child = fork(path.join(__dirname, '../auth/child.js'), { timeout: 60 * 1000 });
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
    openLoginPage();
  });
};

const postAccessToken = async (code) => {
  try {
    const url = `${LOGIN_BASE_URL}/api/oauth/token`;
    const { data } = await axios.post(
      url,
      `client_id=${CLIENT_ID}&code=${code}&grant_type=authorization_code&redirect_uri=${REDIRECT_URI}`,
      {
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded'
        }
      }
    );

    set({
      [PARTNER_KEYS.SESSION_ID]: data.session_id,
      [PARTNER_KEYS.EXPIRES_IN]: data.expires_in,
      [PARTNER_KEYS.LOGIN_TIMESTAMP]: new Date().getTime()
    });
  } catch (e) {
    console.log(e);
    log.error(chalk.red('\n✗ Failed to post access token'));
    process.exit(-1);
  }
};

const login = async () => {
  let spinner;
  try {
    spinner = ora(`Waiting logging in to ${chalk.green(PARNTER_URL)} \r`).start();

    const code = await getCode(PARNTER_URL);
    await postAccessToken(code);

    spinner && spinner.succeed();
    log.info('\n', chalk.green(`Logged into ${PARNTER_URL} successfully!`), '\n');
    return true;
  } catch (e) {
    spinner && spinner.fail();
    log.error(chalk.red(`✗ Logged into ${PARNTER_URL} failed`));
  }
};

const loginAndChoose = async () => {
  (await login()) && (await choosePartner()) && (await chooseApp());
};

const checkAndLogin = async () => {
  if (!isLogin()) {
    log.info('\n', chalk.red('Your identity has expired or you are not logged in, please log in first!'), '\n');
    empty();
    await stop(1000);
    await loginAndChoose();
    await stop(1000);
  }
  return true;
};

module.exports = {
  loginIntoPartner: loginAndChoose,
  checkAndLogin
};
