const { checkAndLogin } = require('./login');
const { build } = require('./commands/build');
const { deploy } = require('./commands/deploy');
const { generate } = require('./commands/generate');
const { publish } = require('./commands/publish');
const { choosePartner } = require('./inquirers/choose-partner');
const { chooseApp } = require('./inquirers/choose-app');

const generateExtension = async () => {
  (await checkAndLogin()) && (await generate());
};

const deployExtension = async () => {
  (await checkAndLogin()) && (await deploy());
};

const publishExtension = async () => {
  (await checkAndLogin()) && (await publish());
};

const retry = async (options) => {
  if (options.partner) {
    (await checkAndLogin()) && (await choosePartner()) && (await chooseApp());
  }

  if (options.app) {
    (await checkAndLogin()) && (await chooseApp());
  }
};

module.exports = {
  generateExtension,
  deployExtension,
  publishExtension,
  buildExtension: build,
  retry
};
