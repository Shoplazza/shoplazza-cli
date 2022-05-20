const { login } = require('../login');
const axios = require('axios');
const { get, empty } = require('../../db/user');
const MockAdapter = require('axios-mock-adapter');
const { SSO_AUTH_URL, ACCOUNT_URL } = require('../../config');

jest.mock('../../db');
jest.mock('../../auth/getCode');
jest.mock('inquirer', () => ({
  prompt: () => Promise.resolve({ confirm: 'Yes' })
}));
jest.mock('ora', () => () => ({
  start: () => ({
    stop: () => {}
  })
}));

describe('login', () => {
  let mock;

  beforeEach(() => {
    mock = new MockAdapter(axios);
    empty();
  });

  afterEach(() => {
    mock.reset();
    empty();
  });

  it('success', async () => {
    mock.onPost(`${SSO_AUTH_URL}/api/oauth/token`).replyOnce(200, {
      access_token: 'access_token',
      session_id: 'session_id'
    });

    mock.onGet(`${SSO_AUTH_URL}/api/sso/current/users`).replyOnce(200, {
      users: [
        {
          user_id: 'user_id'
        }
      ]
    });

    mock.onPost(`${ACCOUNT_URL}/api/accounts/store/token`).replyOnce(200, {
      access_token: 'exchange_token'
    });

    console.log = jest.fn;
    await login({
      store: 'developer.myshoplaza.com'
    });
    expect(get('access_token')).toBe('access_token');
    expect(get('session_id')).toBe('session_id');
    expect(get('user_id')).toBe('user_id');
    expect(get('exchange_token')).toBe('exchange_token');
    expect(get('store_domain')).toBe('developer.myshoplaza.com');
  });

  it('failed', async () => {
    const mockExit = jest.spyOn(process, 'exit').mockImplementation((number) => {
      throw new Error('process.exit: ' + number);
    });

    mock.onPost(`${SSO_AUTH_URL}/api/oauth/token`).replyOnce(500, {});
    await login({
      store: 'developer-failed.myshoplaza.com'
    });
    expect(mockExit).toHaveBeenCalledWith(-1);

    expect(get('access_token')).toBe(null);
    expect(get('session_id')).toBe(null);
    expect(get('user_id')).toBe(null);
    expect(get('exchange_token')).toBe(null);
    expect(get('store_domain')).toBe(null);
  });
});
