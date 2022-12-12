const chalk = require('chalk');
const ora = require('ora');
const fs = require('fs-extra');
const path = require('path');

const { zipTheme } = require('../../utils');
const { line, done } = require('../log');
const { buildThemeAssets } = require('../extensions/theme-app');

const isFileExist = (path) => {
  return fs.existsSync(path);
};

const clearManifest = async () => {
  try {
    if (isFileExist('./assets-manifest.json') && isFileExist('./assets')) {
      const mainfest = JSON.parse(fs.readFileSync('./assets-manifest.json', { encoding: 'utf-8' }));
      await Promise.all(
        Object.values(mainfest).map((path) => {
          if (isFileExist(path)) {
            return fs.unlink(path);
          }
          return Promise.resolve(true);
        })
      );
      await fs.unlink('./assets-manifest.json');
    }
    return true;
  } catch (e) {
    console.log(e);
  }
};

const getZipName = () => {
  return path.basename(process.cwd());
};

const generateZip = async (name) => {
  const zipPath = `${process.cwd()}/${name}.zip`;
  isFileExist(zipPath) && (await fs.unlink(zipPath));
  zipTheme(process.cwd(), name);
};

const zipApp = async () => {
  const spinner = ora(chalk.cyan('Building your app and generate zip package ...')).start();
  try {
    const zipName = getZipName();
    await generateZip(zipName);
    spinner.succeed(chalk.cyan(`Success to build your app and generate zip package: ${chalk.green(`${zipName}.zip`)}`));
    return true;
  } catch (e) {
    spinner.fail(e.message);
  }
};

const build = async () => {
  line();
  (await clearManifest()) && (await buildThemeAssets('./assets')) && (await zipApp());
  done();
  line();
};

module.exports = {
  build,
  getZipName
};
