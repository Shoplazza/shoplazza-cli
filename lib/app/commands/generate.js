const inquirer = require('inquirer');
const { set, PARTNER_KEYS } = require('../db/partner');
const { themeAppHandler, checkoutUIHandler } = require('../extensions');
const { line } = require('../log');

const EXTENSION_TYPES = {
  CHECKOUT_UI: 'checkout_ui',
  THEME_APP: 'theme_app'
};

const generate = async () => {
  line();
  const answer = await inquirer.prompt([
    {
      type: 'list',
      name: 'extensionType',
      message: 'Choose your extension type ↓',
      default: EXTENSION_TYPES.THEME_APP,
      prefix: '*',
      choices: [
        {
          name: 'Theme App Extension',
          value: EXTENSION_TYPES.THEME_APP
        }
      ]
    },
    {
      type: 'input',
      name: 'extensionName',
      message: 'Input your extesion name: ',
      default: 'my-extension',
      prefix: '*'
    }
  ]);
  line();

  set({ [PARTNER_KEYS.EXTENSION_TYPE]: answer.extensionType });

  if (answer.extensionType === EXTENSION_TYPES.THEME_APP) {
    await themeAppHandler(answer.extensionName);
  } else if (answer.extensionType === EXTENSION_TYPES.CHECKOUT_UI) {
    await checkoutUIHandler(answer.extensionName);
  }

  line();
};

module.exports = {
  generate
};
