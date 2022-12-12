const chalk = require('chalk');
const inquirer = require('inquirer');
const Sentry = require('@sentry/node');
const ora = require('ora');
const { get } = require('../db/user');
const { hasBeenSetAnalytics, setAnalyticsConfig } = require('../db/analytics');
const log = require('../log');
const { getShopDetail } = require('../openAPI/api');
const getCode = require('../auth/getCode');
const { postAccessToken, getUserInfo, postExchangeToken } = require('../auth');
const { loginIntoPartner } = require('../app/login');

let spinner;

const checkAndLogin = async (store) => {
  if (get('exchange_token')) {
    if (get('store_domain') === store) {
      try {
        await getShopDetail({ ignoreLogError: true });
        log.info(`${chalk.green('✓')} Already logged in to ${chalk.green(store)}`);
        return;
      } catch (err) {
        // Ignore
      }
    } else {
      try {
        await postExchangeToken(store, { ignoreLogError: true });
        log.info(`Logged into ${chalk.green(get('store_domain'))}`);
        return;
      } catch (err) {
        // Ignore
      }
    }
  }

  const code = await getCode(store);
  spinner = ora(`Logging in to ${chalk.green(store)}`).start();
  log.info(`${chalk.green('✓')} Initiating authentication`);
  await postAcxcessToken(code, store);
  await getUserInfo(store);
  await postExchangeToken(store);
  spinner?.stop?.();
  log.info(`${chalk.green('✓')} Finalizing authentication`);
  log.info(`Logged into ${chalk.green(get('store_domain'))}`);
  requestAnalyticsIfNeeded();
};

const requestAnalyticsIfNeeded = async () => {
  const user_id = get('user_id');
  if (hasBeenSetAnalytics(user_id)) return;
  try {
    const answers = await inquirer.prompt([
      {
        name: 'confirm',
        type: 'list',
        message: `Are you sure you want to enable usage reporting?`,
        choices: ['Yes', 'No']
      }
    ]);
    setAnalyticsConfig({
      user_id,
      enabled: answers.confirm === 'Yes' ? 1 : 0
    });
  } catch (error) {
    log.error(chalk.red('✗ Failed to authenticate'));
    Sentry.captureException(err);
  }
};

const loginIntoStore = async (store) => {
  try {
    if (store) {
      await checkAndLogin(store);
    } else {
      const answers = await inquirer.prompt([
        {
          name: 'store',
          type: 'input',
          message: 'The store domain (Eg: developer.myshoplaza.com)'
        }
      ]);
      if (answers.store) {
        if (!answers.store.includes('myshoplaza.com') || !answers.store.includes('myshoplaza.com')) {
          log.error(
            chalk.red(
              `✗ Invalid store provided ${answers.store}. Please provide the store in the following format: developer.myshoplaza.com`
            )
          );
          return;
        }
        await checkAndLogin(answers.store);
      } else {
        log.error(chalk.red(`✗ Please input the store domain.`));
        return;
      }
    }
  } catch (error) {
    spinner?.stop?.();
    log.error(chalk.red(`✗ Failed to authenticate`));
    Sentry.captureException(error);
  }
};

exports.login = async (options) => {
  if (options.partner) {
    await loginIntoPartner();
    return;
  } else {
    await loginIntoStore(options.store);
  }
};
