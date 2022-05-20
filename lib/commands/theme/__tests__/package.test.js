const chalk = require('chalk');
const path = require('path');
const fs = require('fs-extra');
const { packageCommand } = require('../package');
const { fstat } = require('fs');

describe('theme package', () => {
  it('zip theme', async () => {
    process.chdir(path.join(process.cwd(), '/fixtures'));
    console.log = jest.fn();
    await packageCommand();
    expect(console.log.mock.calls[0][0]).toBe(`${chalk.green('✓')} Theme packaged in TestLifeStyle-1.0.zip`);
    expect(fs.existsSync(path.join(process.cwd(), 'TestLifeStyle-1.0.zip'))).toBe(true);
    fs.rmSync(path.join(process.cwd(), 'TestLifeStyle-1.0.zip'));
    process.chdir(path.join(process.cwd(), '../'));
  });
});
