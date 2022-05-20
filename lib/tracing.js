const axios = require('axios');
const Sentry = require('@sentry/node');
const Tracing = require('@sentry/tracing');

const map = new Map();
const generatorTransactionName = (config) => {
  try {
    return `${config.method.toUpperCase()}-${config.url}-${JSON.stringify(config.params)}-${JSON.stringify(
      config.data
    )}`;
  } catch (err) {
    Sentry.captureException(err);
    return `${config.method.toUpperCase()}-${config.url}`;
  }
};

axios.interceptors.request.use((config) => {
  try {
    Sentry.configureScope((scope) => {
      const transaction = Sentry.startTransaction({
        op: 'transaction',
        name: generatorTransactionName(config)
      });
      map.set(generatorTransactionName(config), transaction);
      scope.setSpan(transaction);
    });
  } catch (err) {
    Sentry.captureException(err);
  }

  return config;
});

axios.interceptors.response.use(
  function (response) {
    try {
      const key = generatorTransactionName(response.config);
      const transaction = map.get(key);
      transaction.finish();
      map.delete(key);
    } catch (err) {
      Sentry.captureException(err);
    }
    return response;
  },
  function (error) {
    Sentry.captureException(error);
    return Promise.reject(error);
  }
);
