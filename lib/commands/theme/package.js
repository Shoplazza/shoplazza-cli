const path = require('path');
const fs = require('fs-extra');
const chalk = require('chalk');
const Sentry = require('@sentry/node');
const log = require('../../log');
const { zipTheme } = require('../../utils');

const checkAndZipThemes = () => {
  const configPath = path.join(process.cwd(), 'config/settings_schema.json');
  if (!fs.pathExistsSync(configPath)) {
    log.error(chalk.red('✗ Provide a config/settings_schema.json to package your theme'));
    return;
  }
  const config = fs.readJSONSync(configPath);
  const themeInfo = config.find((section) => section.name === 'theme_info');
  const { theme_name, theme_version = '' } = themeInfo;
  if (!theme_name) {
    log.error(chalk.red('✗ Provide a theme_info.theme_name configuration in config/settings_schema.json'));
    return;
  }
  const zipName = `${theme_name}${theme_version ? `-${theme_version}` : ''}`;
  const zipPath = path.join(process.cwd(), `${zipName}.zip`);
  fs.removeSync(zipPath);
  zipTheme(process.cwd(), zipName);
  return { theme_name, theme_version, zipName, zipPath };
};

exports.checkAndZipThemes = checkAndZipThemes;
exports.packageCommand = () => {
  try {
    const { zipName } = checkAndZipThemes();
    log.info(`${chalk.green('✓')} Theme packaged in ${zipName}.zip`);
  } catch (error) {
    log.error(chalk.red(`✗ Failed to package theme`));
    Sentry.captureException(error);
  }
};
