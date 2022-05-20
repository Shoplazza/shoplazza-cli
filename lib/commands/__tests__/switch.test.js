const axios = require('axios');
const chalk = require('chalk');
const switchCommand = require('../switch');
const { get, set, empty } = require('../../db/user');
const MockAdapter = require('axios-mock-adapter');
const { ACCOUNT_URL } = require('../../config');

jest.mock('../../db');

describe('switch', () => {
  let mock;

  beforeEach(() => {
    mock = new MockAdapter(axios);
    empty();
    set({ access_token: 'access_token' });
  });

  afterEach(() => {
    mock.reset();
    empty();
  });

  it('switch success', async () => {
    mock.onPost(`${ACCOUNT_URL}/api/accounts/store/token`).replyOnce(200, {
      access_token: 'exchange_token'
    });
    console.log = jest.fn;
    await switchCommand({
      store: 'developer.myshoplaza.com'
    });
    expect(get('access_token')).toBe('access_token');
    expect(get('exchange_token')).toBe('exchange_token');
  });

  it('switch failed', async () => {
    mock.onPost(`${ACCOUNT_URL}/api/accounts/store/token`).replyOnce(500, {});
    console.log = jest.fn;
    await switchCommand({
      store: 'developer-failed.myshoplaza.com'
    });
    expect(get('access_token')).toBe(null);
    expect(get('exchange_token')).toBe(null);
  });
});
