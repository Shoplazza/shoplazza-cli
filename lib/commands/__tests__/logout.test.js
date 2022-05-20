const logout = require('../logout');
const { get, set, empty } = require('../../db/user');

jest.mock('../../db');

describe('logout', () => {
  beforeEach(() => {
    empty();
  });

  it('success', async () => {
    set({
      user_id: 'user_id',
      session_id: 'session_id',
      access_token: 'access_token',
      exchange_token: 'exchange_token'
    });
    expect(get('access_token')).toBe('access_token');
    expect(get('session_id')).toBe('session_id');
    expect(get('user_id')).toBe('user_id');
    expect(get('exchange_token')).toBe('exchange_token');
    console.log = jest.fn;
    logout();
    expect(get('access_token')).toBe(null);
    expect(get('session_id')).toBe(null);
    expect(get('user_id')).toBe(null);
    expect(get('exchange_token')).toBe(null);
  });
});
