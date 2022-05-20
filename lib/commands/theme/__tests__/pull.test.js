const chalk = require('chalk');
const fs = require('fs-extra');
const path = require('path');
const MockAdapter = require('axios-mock-adapter');
const AdmZip = require('adm-zip');
const pullCommand = require('../pull');
const openAPI = require('../../../openAPI');
const { set, empty } = require('../../../db/user');

jest.mock('ora', () => () => ({
  start: () => ({
    stop: () => {}
  })
}));
jest.mock('../../../db');
jest.mock('../../../openAPI/index');

describe('theme pull', () => {
  let mock;

  beforeEach(() => {
    set({
      store_domain: 'developer.myshoplaza.com'
    });
    mock = new MockAdapter(openAPI);
  });

  afterEach(() => {
    mock.reset();
    empty();
  });

  it('pull success', async () => {
    const zip = new AdmZip();
    zip.addLocalFolder(path.join(process.cwd(), '/fixtures'), 'test');
    zip.writeZip(path.join(process.cwd(), 'test.zip'));
    fs.ensureDirSync(path.join(process.cwd(), '/fixtures/theme'));

    mock
      .onGet(`https://developer.myshoplaza.com/openapi/2020-07/themes/theme_id/download`)
      .replyOnce(200, fs.createReadStream(path.join(process.cwd(), 'test.zip')));

    fs.rmSync(path.join(process.cwd(), 'test.zip'));

    process.chdir(path.join(process.cwd(), '/fixtures/theme'));

    console.log = jest.fn();
    await pullCommand({
      theme: 'theme_id'
    });

    expect(console.log.mock.calls[0][0]).toBe(chalk.green(`✓ Theme pulled successfully`));
    expect(fs.existsSync(path.join(process.cwd(), '/config/settings_schema.json'))).toBe(true);
    fs.rmSync(process.cwd(), { recursive: true });

    process.chdir(path.join(process.cwd(), '../../'));
  });

  it('pull failed', async () => {
    mock.onGet(`https://developer.myshoplaza.com/openapi/2020-07/themes/theme_id/download`).replyOnce(500);

    console.log = jest.fn();
    await pullCommand({
      theme: 'theme_id'
    });

    expect(console.log.mock.calls[0][0]).toBe(chalk.red(`✗ Failed to pull theme`));
  });
});
