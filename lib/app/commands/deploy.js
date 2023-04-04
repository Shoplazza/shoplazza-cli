const axios = require('axios');
const ora = require('ora');
const chalk = require('chalk');
const fs = require('fs-extra');
const SparkMD5 = require('spark-md5');
const FormData = require('form-data');
const path = require('path');

const { PARNTER_URL } = require('../constants');
const { getValue, PARTNER_KEYS, getApp, set } = require('../db/partner');
const { getZipName } = require('./build');
const { inputVersion } = require('../inquirers/version');
const { line, done } = require('../log');

const getZipPath = () => {
  const spinner = ora(chalk.cyan('Finding your built zip ...')).start();
  try {
    const zipName = getZipName();
    const filename = fs.readdirSync(process.cwd()).find((name) => {
      const extname = path.extname(name).replace('.', '');
      return extname === 'zip' && name.startsWith(zipName);
    });

    if (!filename) {
      throw new Error('not zip');
    }

    const fileInfo = fs.statSync(`${process.cwd()}/${filename}`);

    spinner.succeed(
      chalk.cyan(
        `Success to find your zip: ${chalk.green(`${filename} ${(fileInfo.size / (1024 * 1024)).toFixed(2)}M`)}`
      )
    );
    return path.resolve(process.cwd(), filename);
  } catch (e) {
    spinner.fail(chalk.red('Failed to find your zip, be sure you have built app in root dir!'));
  }
};

const getBufferAndMd5 = async (filePath) => {
  const spinner = ora(chalk.cyan('Begin to read zip and generate md5 code ...')).start();

  try {
    return await new Promise((resolve, reject) => {
      const spark = new SparkMD5.ArrayBuffer();
      let buffers = [];

      const rs = fs.createReadStream(filePath, { autoClose: true });
      rs.on('data', (data) => {
        buffers.push(data);
        spark.append(data);
      });

      rs.on('end', () => {
        const md5 = spark.end();
        const completedBuffer = Buffer.concat(buffers);
        spinner.succeed(chalk.cyan('Success to analyse zip'));
        resolve([completedBuffer, md5 + '.zip']);
      });

      rs.on('error', (err) => {
        reject(err);
      });
    });
  } catch (e) {
    spinner.fail(chalk.red('Failed to analyse zip, please try again!'));
  }
};

const getSign = async () => {
  const spinner = ora(chalk.cyan('Waiting get file sign ...')).start();

  try {
    const app = getApp();
    const partnerId = getValue(PARTNER_KEYS.PARTNER_ID);
    const sessionId = getValue(PARTNER_KEYS.SESSION_ID);

    const url = `${PARNTER_URL}/api/partner/apps/${app.uid}/theme_extensions/file/signv2`;
    const res = await axios.get(url, {
      headers: {
        Cookie: `awesomev2=${sessionId}`,
        'x-shoplazza-partner-id': partnerId
      }
    });
    spinner.succeed(chalk.cyan('Success to get file sign'));
    return res.data;
  } catch (e) {
    spinner.fail(chalk.red(e.message || e));
  }
};

const deployOss = async () => {
  const path = getZipPath();
  if (path) {
    const [buffer, md5] = await getBufferAndMd5(path);
    const signData = await getSign();
    if (!signData) {
      return;
    }

    const spinner = ora(chalk.cyan('Deploying your zip to cdn ...')).start();

    try {
      const formData = new FormData();
      formData.append('policy', signData.policy);
      formData.append('OSSAccessKeyId', signData.access_id);
      formData.append('success_action_status', 200);
      formData.append('signature', signData.sign);
      formData.append('x-oss-forbid-overwrite', 'true');
      formData.append('key', md5);
      formData.append('file', buffer);

      const url = `https:${signData.write_host}/`;
      const res = await axios.post(url, formData, {
        global: true,
        maxContentLength: 100000000,
        maxBodyLength: 1000000000
      });

      if (res.status !== 200) {
        throw new Error(`${res.status} ${resstatusText}`);
      }
      spinner.succeed();
      return md5;
    } catch (e) {
      // 409 repeat filename
      if (e?.response?.status === 409) {
        spinner.succeed();
        return md5;
      }

      spinner.fail();
      console.log(chalk.red(e.message || e));
    }
  }
};

const deployPartner = async () => {
  const md5 = await deployOss();
  if (!md5) {
    return;
  }

  const spinner = ora(chalk.cyan('Deploying your zip to PARTNER ...')).start();
  try {
    const app = getApp();
    if (!app) {
      spinner.fail(chalk.red('Please choose your partner first!'));
      return;
    }

    const partnerId = getValue(PARTNER_KEYS.PARTNER_ID);
    const url = `${PARNTER_URL}/api/partner/apps/${app.uid}/theme_extensions`;
    const res = await axios.put(
      url,
      {
        title: getZipName(),
        name: getZipName(),
        file_name: md5
      },
      {
        headers: {
          cookie: `awesomev2=${getValue(PARTNER_KEYS.SESSION_ID)};`,
          'x-shoplazza-partner-id': partnerId
        }
      }
    );

    set({ [PARTNER_KEYS.EXTENSION_ID]: res.data.extension_id });
    spinner.succeed();
    return true;
  } catch (e) {
    spinner.fail();
    console.log(e.message || e);
    console.log(chalk.red(JSON.stringify(e.response.data)));
  }
};

const createVersion = async (version) => {
  const spinner = ora(chalk.cyan('Creating your version task ...')).start();

  try {
    const app = getApp();
    const extensionId = getValue(PARTNER_KEYS.EXTENSION_ID);
    const partnerId = getValue(PARTNER_KEYS.PARTNER_ID);

    const url = `${PARNTER_URL}/api/partner/apps/${app.uid}/theme_extensions/${extensionId}/version_tasks`;
    await axios.post(
      url,
      {
        version
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
    spinner.fail();
    console.log(chalk.red(e.message || e));
    console.log(chalk.red(JSON.stringify(e.response?.data)));
  }
};

const deploy = async () => {
  line();
  ora(chalk.cyan('Deploy Begin')).succeed();
  let version;
  (await deployPartner()) && (version = await inputVersion()) && (await createVersion(version)) && done();
  line();
};

module.exports = {
  deploy
};
