const path = require('path');
const fs = require('fs-extra');
const { zipTheme, unzipTheme, formatThemeList, sleep, getThemeFilenameTypeAndLocation } = require('../utils');

describe('utils', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('func getThemeFilenameTypeAndLocation', () => {
    expect(getThemeFilenameTypeAndLocation('~/xxx/demo/assets/header.js')).toMatchObject({
      type: 'assets',
      location: 'header.js'
    });

    expect(getThemeFilenameTypeAndLocation('~/xxx/demo/layout/theme.liquid')).toMatchObject({
      type: 'layout',
      location: 'theme.liquid'
    });
  });

  it('func sleep', async () => {
    const spy = jest.fn();
    sleep(100).then(spy); // <= resolve after 100ms

    jest.advanceTimersByTime(20); // <= advance less than 100ms
    await Promise.resolve(); // let any pending callbacks in PromiseJobs run
    expect(spy).not.toHaveBeenCalled(); // SUCCESS

    jest.advanceTimersByTime(80); // <= advance the rest of the time
    await Promise.resolve(); // let any pending callbacks in PromiseJobs run
    expect(spy).toHaveBeenCalled(); // SUCCESS
  });

  it('func formatThemeList', () => {
    expect(
      formatThemeList(
        [
          { id: '111', name: 'aaa' },
          { id: '222', name: 'bbb' }
        ],
        '222'
      )
    ).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ id: '111', name: expect.stringContaining('[unpublished]'), value: '111' }),
        expect.objectContaining({ id: '222', name: expect.stringContaining('[live]'), value: '222' })
      ])
    );
  });

  it('func zipTheme', () => {
    zipTheme(path.join(__dirname, '../../fixtures'), 'test');
    expect(fs.existsSync(path.resolve('./test.zip'))).toBe(true);
    fs.removeSync(path.resolve('./test.zip'));
  });

  it('func zipTheme', () => {
    zipTheme(path.join(__dirname, '../../fixtures'), 'test');
    unzipTheme(path.resolve('./test.zip'), path.join(__dirname, 'test'));
    expect(fs.existsSync(path.resolve('./test.zip'))).toBe(false);
    expect(fs.existsSync(path.join(__dirname, 'test'))).toBe(true);
    fs.removeSync(path.join(__dirname, 'test'));
  });
});
