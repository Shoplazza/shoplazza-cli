const store = require('../store');
const chalk = require('chalk');
const { set, empty } = require('../../db/user');
const MockAdapter = require('axios-mock-adapter');
const openAPI = require('../../openAPI');

jest.mock('../../db');
jest.mock('../../openAPI');

describe('store', () => {
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

  it('success', async () => {
    mock.onGet(`https://developer.myshoplaza.com/openapi/2020-07/shop`).replyOnce(200, {
      shop: {
        domain: 'developer.myshoplaza.com'
      }
    });
    console.log = jest.fn();
    await store();
    expect(console.log.mock.calls[0][0]).toBe(
      `You're currently logged into ${chalk.green('developer.myshoplaza.com')}`
    );
  });

  it('failed', async () => {
    mock.onGet(`https://developer.myshoplaza.com/openapi/2020-07/shop`).replyOnce(500);
    console.log = jest.fn();
    await store();
    expect(console.log.mock.calls[0][0]).toBe(chalk.red(`✗ Failed to get store`));
  });
});
