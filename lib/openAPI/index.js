const axios = require('axios');
const fs = require('fs-extra');
const chalk = require('chalk');
const Sentry = require('@sentry/node');
const { get, set } = require('../db/user');
const { getThemeFilenameTypeAndLocation } = require('../utils');
const { postExchangeToken } = require('../auth');
const log = require('../log');

const openAPI = axios.create({
  baseURL: `https://${get('store_domain')}/openapi/2020-07`,
  headers: { 'Access-Token': get('exchange_token') }
});

openAPI.interceptors.request.use((config) => {
  if (!get('store_domain')) {
    log.error(
      chalk.red(
        `\n✗ No store found. Please run ${chalk.cyan('shoplazza login --store STORE')} to login to a specific store`
      )
    );
    process.exit(-1);
  }
  return config;
});

openAPI.interceptors.response.use(
  function (response) {
    return response;
  },
  function (error) {
    // Token expired will return a 400 error
    if (error?.response?.status === 400) {
      set({
        exchange_token: ''
      });
      return postExchangeToken(get('store_domain'), { ignoreLogError: error.config.ignoreLogError }).then(() => {
        error.config.headers['Access-Token'] = get('exchange_token');
        return openAPI(error.config);
      });
    }
    return Promise.reject(error);
  }
);

module.exports = openAPI;
