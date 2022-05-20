const MockAdapter = require('axios-mock-adapter');
const chalk = require('chalk');
const deleteCommand = require('../delete');
const openAPI = require('../../../openAPI');
const { set, empty } = require('../../../db/user');

jest.mock('inquirer', () => ({
  prompt: () => Promise.resolve({ confirm: 'Yes' })
}));
jest.mock('../../../db');
jest.mock('../../../openAPI/index');

describe('delete theme', () => {
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

  it('delete success', async () => {
    mock.onGet(`https://developer.myshoplaza.com/openapi/2020-07/themes/theme_id`).replyOnce(200, {
      data: { name: 'test' }
    });
    mock.onDelete(`https://developer.myshoplaza.com/openapi/2020-07/themes/theme_id`).replyOnce(200);

    console.log = jest.fn();
    await deleteCommand({ theme: 'theme_id' });
    expect(console.log.mock.calls[0][0]).toBe(chalk.green(`✓ test (theme_id) theme deleted`));
  });

  it('delete failed', async () => {
    mock.onGet(`https://developer.myshoplaza.com/openapi/2020-07/themes/theme_id`).replyOnce(200, {
      data: { name: 'test' }
    });
    mock.onDelete(`https://developer.myshoplaza.com/openapi/2020-07/themes/theme_id`).replyOnce(500);

    console.log = jest.fn();
    await deleteCommand({ theme: 'theme_id' });
    expect(console.log.mock.calls[0][0]).toBe(chalk.red(`✗ Failed to delete theme`));
  });
});
