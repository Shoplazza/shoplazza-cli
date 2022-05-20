const chalk = require('chalk');
const path = require('path');
const MockAdapter = require('axios-mock-adapter');
const { pushCommand } = require('../push');
const openAPI = require('../../../openAPI');
const { set, empty } = require('../../../db/user');

jest.mock('ora', () => () => ({
  start: () => ({
    stop: () => {}
  })
}));
jest.mock('../../../db');
jest.mock('../../../openAPI/index');

describe('theme push', () => {
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

  it('push success', async () => {
    mock.onGet(`https://developer.myshoplaza.com/openapi/2020-07/themes/theme_id`).replyOnce(200, {
      data: { merchant_theme_id: 'merchant_theme_id' }
    });
    mock.onPost(`https://developer.myshoplaza.com/openapi/2020-07/themes/upload`).replyOnce(200, {
      task: { task: { id: 1 } }
    });
    mock.onGet(`https://developer.myshoplaza.com/openapi/2020-07/themes/task/1`).replyOnce(200, {
      task: { status: 1, info: JSON.stringify({ name: 'TestLifeStyle', theme_id: 'theme_id' }) }
    });

    process.chdir(path.join(process.cwd(), '/fixtures'));

    console.log = jest.fn();
    await pushCommand({
      theme: 'theme_id'
    });

    expect(console.log.mock.calls[0][0]).toBe(chalk.green(`✓ The TestLifeStyle theme pushed successfully`));
    process.chdir(path.join(process.cwd(), '../'));
  });

  it('push failed', async () => {
    mock.onGet(`https://developer.myshoplaza.com/openapi/2020-07/themes/theme_id`).replyOnce(500);

    console.log = jest.fn();
    await pushCommand({
      theme: 'theme_id'
    });

    expect(console.log.mock.calls[0][0]).toBe(chalk.red(`✗ Failed to push theme`));
  });
});
