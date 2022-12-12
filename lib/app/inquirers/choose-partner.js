const axios = require('axios');
const inquirer = require('inquirer');
const ora = require('ora');
const chalk = require('chalk');

const { getValue, PARTNER_KEYS, set, get } = require('../db/partner');
const { PARNTER_URL } = require('../constants');
const log = require('../../log');

const requestPartnerList = async () => {
  const spinner = ora(chalk.cyan(`Waiting request your partners ...`)).start();

  try {
    const url = `${PARNTER_URL}/api/partners`;
    const sessionId = getValue(PARTNER_KEYS.SESSION_ID);

    const result = await axios.get(url, {
      headers: {
        Cookie: `awesomev2=${sessionId}`
      }
    });

    if (!result.data.length) {
      spinner.fail(chalk.red(`You haven't any partners yet, please create one at ${PARNTER_URL} fist!`));
      return;
    }

    spinner.succeed();
    return result.data;
  } catch (e) {
    spinner.fail(e.message);
    log.error(
      chalk.red(
        `Failed to get partners, please try to run \n ${chalk.green(
          '   shoplazza app retry -p    '
        )} \nor contact with Shoplazza!`
      )
    );
  }
};

const choosePartner = async () => {
  const list = await requestPartnerList();
  if (!list) {
    return;
  }

  try {
    const answer = await inquirer.prompt([
      {
        type: 'list',
        name: 'partnerId',
        message: 'Choose your partner:',
        default: list[0].partner_id,
        prefix: '*',
        choices: list.map((app) => {
          return {
            name: `${app.partner_id} ${app.business_name}`,
            value: String(app.partner_id)
          };
        })
      }
    ]);

    set({ [PARTNER_KEYS.PARTNER_ID]: answer['partnerId'] });
    return true;
  } catch (e) {
    console.log(e);
  }
};

module.exports = {
  choosePartner
};
