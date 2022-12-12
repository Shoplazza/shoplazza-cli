const chalk = require('chalk');
const ora = require('ora');
const fs = require('fs-extra');
const glob = require('glob');
const repo = require('download-git-repo');
const CleanCSS = require('clean-css');
const UglifyJS = require('uglify-js');

const crypto = require('crypto');
const path = require('path');
const { line } = require('../log');

const rev = (input) => crypto.createHash('md5').update(input).digest('hex').slice(0, 8);
const cleanCSS = new CleanCSS({
  advanced: false,
  aggressiveMerging: false
});

const generateThemeDirectory = async (dirname = './') => {
  const spinner = ora(chalk.cyan(`Waiting generate your theme app project...`)).start();
  try {
    await new Promise((resolve, reject) => {
      repo('Shoplazza/theme-extension-template', dirname, {}, (err) => {
        if (err) {
          reject(JSON.stringify(err));
        } else {
          resolve(true);
        }
      });
    });

    spinner.succeed(chalk.cyan(`Generate app successfully`));
    return true;
  } catch (e) {
    spinner.fail();
    console.log(chalk.red(e));
  }
};

const buildThemeAssets = async (outputDir = './dist', pathPrefix = '.') => {
  line();
  ora(chalk.cyan(`Prepare Build assets ...`)).succeed();

  const manifest = {};

  // build assets css
  const spinner = ora(chalk.cyan(`Build assets ...`)).start();
  try {
    const cssPathnames = glob.sync('./assets/*.css');
    const jsPathnames = glob.sync('./assets/*.js');

    const pathnames = [...cssPathnames, ...jsPathnames];
    for (const pathname of pathnames) {
      const filename = path.basename(pathname);
      const extname = path.extname(pathname);
      const fileContent = fs.readFileSync(pathname, 'utf8');
      const hash = rev(fileContent);

      let minified = '';
      let filenameWithHash = '';
      if (extname.includes('css')) {
        minified = cleanCSS.minify(fileContent);
        if (minified.errors.length) {
          spinner.fail(minified.errors);
          process.exit(-1);
        }
        filenameWithHash = `${filename.split('.')[0]}-${hash}.css`;
        const assetsPath = `${outputDir}/${filenameWithHash}`;
        fs.ensureFileSync(assetsPath);
        fs.writeFileSync(assetsPath, minified.styles);
      } else if (extname.includes('js')) {
        minified = UglifyJS.minify(fileContent);
        if (minified.error) {
          log.error(minified.error);
          process.exit(-1);
        }
        filenameWithHash = `${filename.split('.')[0]}-${hash}.js`;
        const assetsPath = `${outputDir}/${filenameWithHash}`;
        fs.ensureFileSync(assetsPath);
        fs.writeFileSync(assetsPath, minified.code);
      }

      manifest[filename] = pathPrefix + '/assets/' + filenameWithHash;
    }

    fs.writeFileSync(`./assets-manifest.json`, JSON.stringify(manifest));
    spinner.succeed(chalk.cyan(`Build assets done`));
    return true;
  } catch (e) {
    spinner.fail(chalk.red(e.message));
  }

  // build assets js
  const jsSpinner = ora(chalk.cyan(`Build js ...`)).start();
  try {
    jsSpinner.succeed(chalk.cyan(`Build js done`));
  } catch (e) {}
};

module.exports = {
  generateThemeDirectory,
  buildThemeAssets
};
