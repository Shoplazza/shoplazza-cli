const { get, set, empty } = require('../user');
jest.mock('../index');

describe('db/user.js', () => {
  beforeEach(() => {
    empty();
  });

  it('get and set', () => {
    expect(get('access_token')).toBe(null);
    set({
      access_token: 'access_token',
      session_id: 'session_id',
      exchange_token: 'exchange_token'
    });
    expect(get('access_token')).toBe('access_token');
    expect(get('session_id')).toBe('session_id');
    expect(get('exchange_token')).toBe('exchange_token');
  });
});
