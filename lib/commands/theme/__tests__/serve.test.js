const path = require('path');
const fs = require('fs-extra');
const chalk = require('chalk');
const MockAdapter = require('axios-mock-adapter');
const serve = require('../serve');
const openAPI = require('../../../openAPI');
const { set, empty } = require('../../../db/user');

jest.mock('ora', () => () => ({
  start: () => ({
    stop: () => {}
  })
}));
jest.mock('../../../db');
jest.mock('../../../openAPI/index');

describe('theme serve', () => {
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

  it('start dev server', async () => {
    mock.onGet(`https://developer.myshoplaza.com/openapi/2020-07/themes/theme_id`).replyOnce(200, {
      data: { merchant_theme_id: 'merchant_theme_id' }
    });
    mock.onGet(`https://developer.myshoplaza.com/openapi/2020-07/themes/theme_id/doctree`).replyOnce(200, {
      assets: [
        {
          id: '7848a198-9809-4163-91c7-556e6960a0c6',
          location: 'blog.css'
        },
        {
          id: '651a9948-d4e5-4fe0-bcd7-e88349f6eeaf',
          location: 'blog.scss'
        }
      ],
      config: [
        {
          id: '9e7a0930-6f6e-42f3-88ee-31e40bc28fdf',
          location: 'settings_data.json'
        },
        {
          id: '1799d8b6-82dd-4a42-bad4-32d11a98a924',
          location: 'settings_schema.json'
        }
      ]
    });
    mock.onPost(`https://developer.myshoplaza.com/openapi/2020-07/themes/upload`).replyOnce(200, {
      task: { task: { id: 1 } }
    });
    mock.onGet(`https://developer.myshoplaza.com/openapi/2020-07/themes/task/1`).replyOnce(200, {
      task: { status: 1, info: JSON.stringify({ name: 'TestLifeStyle', theme_id: 'theme_id' }) }
    });
    mock.onPost(`https://developer.myshoplaza.com/openapi/2020-07/themes/theme_id/doc`).replyOnce(200);
    mock.onPatch(`https://developer.myshoplaza.com/openapi/2020-07/themes/theme_id/doc`).replyOnce(200);
    mock.onDelete(`https://developer.myshoplaza.com/openapi/2020-07/themes/theme_id/doc`).replyOnce(200);

    process.chdir(path.join(process.cwd(), '/fixtures'));
    console.log = jest.fn();
    const watcher = await serve({ theme: 'theme_id' });

    expect(console.log.mock.calls[0][0]).toBe(
      `Please open this URL in your browser:
        ${chalk.green('https://developer.myshoplaza.com/?preview_theme_id=theme_id')}

        Customize this theme in the Theme Editor, and use 'theme pull' to get the changes:
        ${chalk.green(`https://developer.myshoplaza.com/admin/smart_apps/editor?theme_id=theme_id`)}`.replace(
        /^[^\S\n]+/gm,
        ''
      )
    );
    expect(console.log.mock.calls[1][0]).toBe(`\nListening for file changes ...`);

    fs.writeFileSync(path.join(process.cwd(), 'assets/test.scss'), 'body { color: red; }', 'utf8');
    await new Promise((r) => setTimeout(r, 1000));
    expect(console.log.mock.calls[2][0]).toBe(chalk.green(`[update]: ${path.join(process.cwd(), 'assets/test.scss')}`));
    expect(console.log.mock.calls[3][0]).toBe(
      chalk.cyan(`Updated, please refresh your browser, will continue listening for file changes ...`)
    );
    fs.rmSync(path.join(process.cwd(), 'assets/test.scss'));
    await new Promise((r) => setTimeout(r, 1000));
    expect(console.log.mock.calls[4][0]).toBe(chalk.green(`[remove]: ${path.join(process.cwd(), 'assets/test.scss')}`));

    process.chdir(path.join(process.cwd(), '../'));
    watcher.close();
  });

  it('not a theme directory', async () => {
    mock.onGet(`https://developer.myshoplaza.com/openapi/2020-07/themes/theme_id`).replyOnce(200, {
      data: { merchant_theme_id: 'merchant_theme_id' }
    });
    await serve({ theme: 'theme_id' });
    expect(console.log.mock.calls[0][0]).toBe(
      chalk.red('✗ Provide a config/settings_schema.json to package your theme')
    );
  });
});
