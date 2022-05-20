const chalk = require('chalk');
const path = require('path');
const MockAdapter = require('axios-mock-adapter');
const publishCommand = require('../publish');
const openAPI = require('../../../openAPI');
const { set, empty } = require('../../../db/user');

jest.mock('inquirer', () => ({
  prompt: () => Promise.resolve({ confirm: 'Yes' })
}));
jest.mock('../../../db');
jest.mock('../../../openAPI/index');

describe('theme publish', () => {
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

  it('publish success', async () => {
    mock.onGet(`https://developer.myshoplaza.com/openapi/2020-07/themes/theme_id`).replyOnce(200, {
      data: { name: 'TestLifeStyle', merchant_theme_id: 'merchant_theme_id' }
    });
    mock.onPatch(`https://developer.myshoplaza.com/openapi/2020-07/themes/theme_id/publish`).replyOnce(200);

    process.chdir(path.join(process.cwd(), '/fixtures'));

    console.log = jest.fn();
    await publishCommand({
      theme: 'theme_id'
    });

    expect(console.log.mock.calls[0][0]).toBe(
      chalk.green(`✓ Your theme is now live at https://developer.myshoplaza.com`)
    );
    process.chdir(path.join(process.cwd(), '../'));
  });

  it('publish failed', async () => {
    mock.onGet(`https://developer.myshoplaza.com/openapi/2020-07/themes/theme_id`).replyOnce(200, {
      data: { name: 'TestLifeStyle', merchant_theme_id: 'merchant_theme_id' }
    });
    mock.onPatch(`https://developer.myshoplaza.com/openapi/2020-07/themes/theme_id/publish`).replyOnce(500);

    console.log = jest.fn();
    await publishCommand({
      theme: 'theme_id'
    });

    expect(console.log.mock.calls[0][0]).toBe(chalk.red(`✗ Failed to publish theme`));
  });
});
