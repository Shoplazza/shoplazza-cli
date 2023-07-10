const path = require('path');
const process = require('process');
const fs = require('fs-extra');
const chalk = require('chalk');
const inquirer = require('inquirer');
const Sentry = require('@sentry/node');
const execa = require('execa');
const ora = require('ora');
const log = require('../../log');

const createDirAndCloneTheme = async (directory) => {
  const themeDir = path.join(process.cwd(), directory);
  if (fs.existsSync(themeDir)) {
    log.error(chalk.red(`✗ Destination path ${directory} already exists and is not an empty directory.`));
    return;
  }
  fs.mkdirSync(themeDir);
  const spinner = ora(chalk.cyan(`Cloning https://github.com/Shoplazza/Nova-2023 into ${directory}…`)).start();
  await execa('git', ['clone', 'https://github.com/Shoplazza/Nova-2023', directory]);
  fs.rmSync(path.join(process.cwd(), `${directory}/.git`), { recursive: true });
  spinner.stop();
  log.info(chalk.green(`✓ Cloned into ${directory}`));
  log.info(`Please run ${chalk.green(`cd ${directory}`)}`);
};

module.exports = async (options) => {
  try {
    if (options.name) {
      await createDirAndCloneTheme(options.name);
      return;
    }
    inquirer
      .prompt([
        {
          name: 'name',
          type: 'input',
          message: 'Theme directory name'
        }
      ])
      .then((answers) => {
        if (answers.name) {
          createDirAndCloneTheme(answers.name);
        } else {
          log.error(chalk.red(`✗ Please input theme directory name.`));
        }
      });
  } catch (error) {
    log.error(chalk.red(`✗ Failed to init theme`));
    Sentry.captureException(error);
  }
};
