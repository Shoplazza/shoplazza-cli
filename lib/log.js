const report = require('./report');

module.exports = {
  error: (...args) => {
    console.log(...args);
    report('cli_usage_error', {
      args: args
    });
  },
  info: (...args) => {
    console.log(...args);
  }
};
