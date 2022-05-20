const pkg = require('../../package.json');
const log = require('../log');

module.exports = () => {
  log.info(pkg.version);
};
