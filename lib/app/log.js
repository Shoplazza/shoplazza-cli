const ora = require('ora');
const chalk = require('chalk');

const done = () => {
  ora(chalk.cyan(`Done`)).succeed();
};

const line = () => {
  console.log('\b');
};

module.exports = {
  done,
  line
};
