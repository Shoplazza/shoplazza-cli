const axios = require('axios');
const inquirer = require('inquirer');
const ora = require('ora');
const chalk = require('chalk');

const { getValue, PARTNER_KEYS, set, get } = require('../db/partner');
const { PARNTER_URL } = require('../constants');

const requestAppList = async () => {
  const spinner = ora(chalk.cyan(`Waiting request your apps ...`)).start();

  try {
    const partnerId = getValue(PARTNER_KEYS.PARTNER_ID);
    if (!partnerId) {
      spinner.fail('You have to choose a partner first!');
      return;
    }

    const url = `${PARNTER_URL}/api/partners/${partnerId}/apps`;
    const result = await axios.get(url, {
      headers: {
        cookie: `awesomev2=${getValue(PARTNER_KEYS.SESSION_ID)};`,
        'x-shoplazza-partner-id': partnerId
      }
    });

    if (!result.data.length) {
      spinner.fail(chalk.red(`You haven't any apps yet, please create one at ${PARNTER_URL} fist!`));
      return;
    }

    spinner.succeed();
    return result.data;
  } catch (e) {
    spinner.fail();
    console.log(
      chalk.red(
        `Failed to get partners, please try to run \n ${chalk.green(
          '   shoplazza app retry -p   '
        )} \nor contact with Shoplazza!`
      )
    );
  }
};

const chooseApp = async () => {
  const list = await requestAppList();
  if (!list) {
    return;
  }

  try {
    const answer = await inquirer.prompt([
      {
        type: 'list',
        name: 'appId',
        message: 'Choose your app:',
        default: list[0].id,
        prefix: '*',
        choices: list.map((app) => {
          return {
            name: app.name,
            value: app.id
          };
        })
      }
    ]);

    const app = list.find((app) => app.id === answer['appId']);
    set({ [PARTNER_KEYS.APP]: JSON.stringify(app) });
    return true;
  } catch (e) {
    console.log(e);
  }
};

module.exports = {
  chooseApp
};
