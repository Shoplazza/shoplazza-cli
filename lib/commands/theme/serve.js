const path = require('path');
const os = require('os');
const fs = require('fs-extra');
const ora = require('ora');
const chalk = require('chalk');
const inquirer = require('inquirer');
const Sentry = require('@sentry/node');
const livereload = require('livereload');
const watch = require('node-watch');
const { get, set } = require('../../db/user');
const { pushThemeFiles } = require('./push');
const { getThemes, getDefaultThemeDetail, deleteFile, updateFile, addFile, getFileList } = require('../../openAPI/api');
const log = require('../../log');
const { THEME_DIRS } = require('../../config');
const { getThemeFilenameTypeAndLocation, formatThemeList } = require('../../utils');

const existInList = (filename, fileList) => {
  const { type, location } = getThemeFilenameTypeAndLocation(filename);
  return fileList[type] && fileList[type].find((item) => item.location == location);
};
const deleteInList = (filename, fileList) => {
  const { type, location } = getThemeFilenameTypeAndLocation(filename);
  fileList[type] = fileList[type].filter((i) => i.location !== location);
};
const addInList = (filename, fileList) => {
  const { type, location } = getThemeFilenameTypeAndLocation(filename);
  fileList[type].push({ location });
};

const startDevServer = async (theme) => {
  const { data: fileList } = await getFileList(theme);
  const server = livereload.createServer({ port: '21647', applyCSSLive: false, applyImgLive: false });
  const watcher = watch(process.cwd(), {
    recursive: true,
    filter: (f, skip) => {
      const { type } = getThemeFilenameTypeAndLocation(f);
      return THEME_DIRS.includes(type);
    }
  });
  return new Promise((resolve, reject) => {
    watcher
      .on('change', async (evt, filename) => {
        try {
          log.info(chalk.green(`[${evt}]: ${filename}`));
          if (
            fs.existsSync(path.join(process.cwd(), filename)) &&
            fs.lstatSync(path.join(process.cwd(), filename)).isDirectory()
          )
            return; // windows change return directory always
          if (evt == 'remove') {
            await deleteFile(theme, filename);
            deleteInList(filename, fileList);
          } else if (!/\/\.\w+/.test(filename)) {
            // path not contain .xxx hidden file/folder
            if (existInList(filename, fileList)) {
              await updateFile(theme, filename);
            } else {
              await addFile(theme, filename);
              addInList(filename, fileList);
              if (fs.readFileSync(filename, 'utf-8')) {
                // addFile api not support post with content
                await updateFile(theme, filename);
              }
            }
          }
          server.refresh();
          log.info(chalk.cyan(`Updated, please refresh your browser, will continue listening for file changes ...`));
        } catch (err) {
          log.error(chalk.red(`✗ ${err.message || 'Unknown error'}`));
        }
      })
      .on('error', (err) => {
        reject(err);
        log.error(chalk.red(`✗ Error `, err));
        Sentry.captureException(err);
      })
      .on('ready', () => {
        resolve(watcher);
        const storeDomain = get('store_domain');
        const url = new URL(`https://${storeDomain}`);
        url.searchParams.set('preview_theme_id', theme);
        log.info(
          `Please open this URL in your browser:
        ${chalk.green(url.href)}

        Customize this theme in the Theme Editor, and use 'theme pull' to get the changes:
        ${chalk.green(`https://${storeDomain}/admin/smart_apps/editor?theme_id=${theme}`)}`.replace(/^[^\S\n]+/gm, '')
        );
        log.info(`\nListening for file changes ...`);
      });
  });
};

module.exports = async (options) => {
  try {
    const storeDomain = get('store_domain');
    if (options.theme) {
      const spinner = ora(chalk.cyan(`Syncing theme files on ${storeDomain}`)).start();
      const { name } = await pushThemeFiles(options.theme);
      set({
        theme_id: options.theme,
        theme_name: `${name} (${options.theme})`
      });
      spinner.stop();
      return startDevServer(options.theme);
    } else {
      const spinner = ora(chalk.cyan(`Fetching theme lists`)).start();
      const [themesRes, defaultThemeRes] = await Promise.all([getThemes(), getDefaultThemeDetail()]);
      spinner.stop();
      const themeList = [defaultThemeRes.data.data, ...themesRes.data.data.themes];
      const choices = [
        { name: 'Create a new unpublished theme', value: '' },
        ...formatThemeList(themeList, defaultThemeRes.data.data.id)
      ];
      const theme_id = get('theme_id');
      const theme_name = get('theme_id');

      if (theme_id && theme_name && themeList.find((theme) => theme.id == theme_id)) {
        choices.unshift({ name: `Use previous selected ${theme_name}`, value: theme_id });
      }

      const answers = await inquirer.prompt([
        {
          name: 'theme',
          type: 'rawlist',
          message: `Select a theme to push ${chalk.cyan('(Choose with ↑ ↓ ⏎)')}`,
          choices: choices
        }
      ]);

      const syncSpinner = ora(chalk.cyan(`Syncing theme files on ${storeDomain}`)).start();
      if (answers.theme) {
        const themeObj = themeList.find((theme) => theme.id == answers.theme);
        set({
          theme_id: themeObj.id,
          theme_name: themeObj.name
        });
        await pushThemeFiles(answers.theme);
      } else {
        const { name, theme_id } = JSON.parse(await pushThemeFiles(answers.theme));
        set({
          theme_id,
          theme_name: name
        });
      }
      syncSpinner.stop();
      return startDevServer(get('theme_id'));
    }
  } catch (error) {
    log.error(chalk.red(`✗ Failed to serve theme`));
    Sentry.captureException(error);
  }
};
