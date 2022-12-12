const axios = require('axios');
const chalk = require('chalk');
const Sentry = require('@sentry/node');
const querystring = require('querystring');
const { get, set, empty } = require('../db/user');
const log = require('../log');
const { REDIRECT_URI } = require('../config');
const { getAccountUrl, getClientId, getSSOAuthUrl } = require('../utils');

exports.postAccessToken = async (code, store) => {
  try {
    const { data } = await axios.post(
      `${getSSOAuthUrl(store)}/api/oauth/token`,
      querystring.stringify({
        client_id: getClientId(store),
        code,
        grant_type: 'authorization_code',
        redirect_uri: REDIRECT_URI
      }),
      {
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded'
        }
      }
    );
    empty();
    set({
      access_token: data.access_token,
      session_id: data.session_id
    });
  } catch (err) {
    log.error(chalk.red('\n✗ Failed to post access token'));
    process.exit(-1);
  }
};

exports.getUserInfo = async (store) => {
  try {
    const { data } = await axios.get(`${getSSOAuthUrl(store)}/api/sso/current/users`, {
      headers: {
        Cookie: `awesomev2=${get('session_id')}`
      }
    });
    const activeUser = data.users.find((user) => user.active) || data.users[0];
    set({
      user_id: activeUser.user_id
    });
  } catch (err) {
    log.error(chalk.red('✗ Failed to get user info'));
    process.exit(-1);
  }
};

exports.postExchangeToken = async (storeDomain, options = {}) => {
  return new Promise(async (resolve, reject) => {
    try {
      const { data } = await axios.post(
        `${getAccountUrl(storeDomain)}/api/accounts/store/token`,
        {
          user_id: get('user_id'),
          domain: storeDomain
        },
        {
          headers: {
            Cookie: `awesomev2=${get('session_id')}`
          }
        }
      );
      set({
        store_domain: storeDomain.replace(/^https?:\/\//, ''),
        exchange_token: data.access_token
      });
      resolve();
    } catch (err) {
      reject(err);
      Sentry.captureException(err);
      empty();
      if (!options.ignoreLogError) {
        if (err?.response?.status === 401) {
          log.error(chalk.red(`\n✗ You are not authorized to edit themes on ${storeDomain}.`));
          log.info(
            chalk.green(
              'Check if your user is activated, has permission to edit themes at the store, and try to re-login.'
            )
          );
        } else {
          log.error(chalk.red(`\n✗ ${storeDomain} is not a valid store.`));
        }
      }
    }
  });
};
