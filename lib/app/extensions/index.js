const { generateThemeDirectory } = require('./theme-app');
const { done } = require('../log');

const themeAppHandler = async (dirname = './') => {
  (await generateThemeDirectory(dirname)) && done();
};

const checkoutUIHandler = async () => {};

module.exports = {
  themeAppHandler,
  checkoutUIHandler
};
