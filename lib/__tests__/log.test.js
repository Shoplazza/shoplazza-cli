const log = require('../log');

describe('log', () => {
  it('log.error method', () => {
    console.log = jest.fn();
    log.error('test error');
    expect(console.log.mock.calls[0][0]).toBe('test error');
  });

  it('log.error info', () => {
    console.log = jest.fn();
    log.error('test info');
    expect(console.log.mock.calls[0][0]).toBe('test info');
  });
});
