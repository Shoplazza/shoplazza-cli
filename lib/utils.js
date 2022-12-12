const path = require('path');
const fs = require('fs-extra');
const AdmZip = require('adm-zip');
const chalk = require('chalk');

const { SSO_AUTH_URL, ACCOUNT_URL, CLIENT_ID, DEV_SSO_AUTH_URL, DEV_ACCOUNT_URL, DEV_CLIENT_ID } = require('./config');

exports.unzipTheme = (zipPath, outputPath) => {
  const zip = new AdmZip(zipPath);
  const zipEntries = zip.getEntries();
  const innerName = zipEntries[0].entryName.split('/')[0];
  zipEntries.forEach((entry) => {
    zip.extractEntryTo(
      entry,
      path.join(outputPath, entry.entryName.replace(`${innerName}`, '').replace(path.basename(entry.entryName), '')),
      false,
      true
    );
  });
  fs.removeSync(zipPath);
};

exports.zipTheme = (filePath, themeName) => {
  const zip = new AdmZip();
  zip.addLocalFolder(filePath, themeName);
  zip.writeZip(`${process.cwd()}/${themeName}.zip`);
};

exports.getThemeFilenameTypeAndLocation = (filename) => {
  const result = filename.split(path.sep);
  const location = result.pop();
  const type = result.pop();
  return {
    type,
    location
  };
};

exports.formatThemeList = (themes, defaultThemeId) => {
  return themes.map((theme) => ({
    ...theme,
    name: `${theme.name} (${theme.id ? chalk.green(theme.id) : ''}) ${
      theme.id === defaultThemeId ? chalk.green('[live]') : chalk.yellow('[unpublished]')
    }`,
    value: theme.id
  }));
};

exports.sleep = (time) => new Promise((resolve) => setTimeout(resolve, time));

exports.getClientId = (store) => {
  return store.includes('dev.') ? DEV_CLIENT_ID : CLIENT_ID;
};

exports.getAccountUrl = (store) => {
  return store.includes('dev.myshoplaza.com') ? DEV_ACCOUNT_URL : ACCOUNT_URL;
};

exports.getSSOAuthUrl = (store) => {
  return store.includes('dev.myshoplaza.com') ? DEV_SSO_AUTH_URL : SSO_AUTH_URL;
};
