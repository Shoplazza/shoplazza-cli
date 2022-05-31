const fs = require('fs-extra');
const path = require('path');
const init = require('../init');
const execa = require('execa');

jest.mock('execa', () => jest.fn());

describe('theme init', () => {
  it('clone repo', async () => {
    jest.setTimeout(20 * 1000);
    if (fs.existsSync(path.join(process.cwd(), 'test'))) {
      fs.rmSync(path.join(process.cwd(), 'test'), { recursive: true });
    }
    expect(fs.existsSync(path.join(process.cwd(), 'test'))).toBe(false);
    console.log = jest.fn;
    await init({ name: 'test' });
    expect(execa).toHaveBeenCalledWith('git', ['clone', 'https://github.com/Shoplazza/LifeStyle', 'test']);
    expect(fs.existsSync(path.join(process.cwd(), 'test'))).toBe(true);
    fs.rmSync(path.join(process.cwd(), 'test'), { recursive: true });
  });
});
