const { default: axios } = require('axios');
const { selectVersion } = require('../inquirers/version');
const { PARNTER_URL } = require('../constants');
const { get, getValue, PARTNER_KEYS } = require('../db/partner');
const ora = require('ora');
const chalk = require('chalk');
const { getZipName } = require('./build');
const { done, line } = require('../log');

const publishVersion = async ({ version, appId, versionId, extensionId }) => {
  const spinner = ora(chalk.cyan('Waiting publish ...'));

  try {
    const partnerId = getValue(PARTNER_KEYS.PARTNER_ID);
    if (!partnerId) {
      spinner.fail(chalk.red('Please choose a partner first!'));
      return;
    }

    const url = `${PARNTER_URL}/api/partner/apps/${appId}/theme_extensions/${extensionId}/publications`;
    await axios.post(
      url,
      {
        name: getZipName(),
        version_id: versionId,
        type: 'enable'
      },
      {
        headers: {
          cookie: `awesomev2=${getValue(PARTNER_KEYS.SESSION_ID)};`,
          'x-shoplazza-partner-id': partnerId
        }
      }
    );

    spinner.succeed();
    return true;
  } catch (e) {
    spinner.fail(e.message || e);
  }
};

const publish = async () => {
  const versionData = await selectVersion();
  await publishVersion(versionData);
  done();
  line();
};

module.exports = {
  publish
};
