const axios = require('axios');
const chalk = require('chalk');
const inquirer = require('inquirer');

const { PARNTER_URL } = require('../constants');
const { getApp, getValue, PARTNER_KEYS } = require('../db/partner');
const { line } = require('../log');

const requestLastVersion = async () => {
  try {
    const app = getApp();
    if (!app) {
      return;
    }
    const extensionId = getValue(PARTNER_KEYS.EXTENSION_ID);
    const partnerId = getValue(PARTNER_KEYS.PARTNER_ID);

    const url = `${PARNTER_URL}/api/partner/apps/${app.uid}/theme_extensions/${extensionId}/last_version`;
    const res = await axios.get(url, {
      headers: {
        cookie: `awesomev2=${getValue(PARTNER_KEYS.SESSION_ID)};`,
        'x-shoplazza-partner-id': partnerId
      },
      validateStatus(status) {
        if (status === 404) {
          return true;
        }
        return status >= 200 && status < 299;
      }
    });

    if (res.data.errors) {
      return;
    }

    return res.data.version;
  } catch (e) {
    console.log(chalk.red(e.message || e));
    console.log(chalk.red(JSON.stringify(e.response?.data)));
  }
};

const requestVersionsData = async (params = {}) => {
  try {
    const app = getApp();
    if (!app) {
      return;
    }
    const extensionId = getValue(PARTNER_KEYS.EXTENSION_ID);
    const partnerId = getValue(PARTNER_KEYS.PARTNER_ID);

    const url = `${PARNTER_URL}/api/partner/apps/${app.uid}/theme_extensions/${extensionId}/versions`;
    const res = await axios.get(url, {
      params,
      headers: {
        cookie: `awesomev2=${getValue(PARTNER_KEYS.SESSION_ID)};`,
        'x-shoplazza-partner-id': partnerId
      },
      validateStatus(status) {
        if (status === 404) {
          return true;
        }
        return status >= 200 && status < 299;
      }
    });

    if (res.data.errors) {
      throw new Error(JSON.stringify(res.data.errors));
    }

    return res.data.data;
  } catch (e) {
    console.log(chalk.red(e.message || e));
    console.log(chalk.red(JSON.stringify(e.response?.data)));
  }
};

const inputVersion = async () => {
  line();
  const lastVersionData = await requestLastVersion({ limit: 1 });
  const hint = lastVersionData ? chalk.gray(`(last version: ${lastVersionData.version})`) : '';

  const answer = await inquirer.prompt([
    {
      type: 'input',
      name: 'extensionVersion',
      message: `${chalk.blue('Input your extension version')} ${hint}:`,
      prefix: `${chalk.green('⭐️')}`
    }
  ]);

  return answer['extensionVersion'];
};

const selectVersion = async () => {
  line();

  const versionDataList = await requestVersionsData();
  if (!versionDataList) {
    console.log(chalk.red('Please deploy a version first!'));
    line();
  }

  const answer = await inquirer.prompt([
    {
      type: 'list',
      name: 'versionData',
      message: `${chalk.blue('Choose a version to publish:')}`,
      default: versionDataList[0].version,
      prefix: `${chalk.blue('🐱')}`,
      choices: versionDataList.map((v) => {
        return {
          name: `${v.version} ${v.published ? chalk.green('published') : ''}`,
          value: {
            version: v.version,
            appId: v.app_id,
            versionId: v.version_id,
            extensionId: v.extension_id
          }
        };
      })
    }
  ]);

  return answer['versionData'];
};

module.exports = {
  inputVersion,
  selectVersion
};
