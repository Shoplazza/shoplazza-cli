const chalk = require('chalk');
const MockAdapter = require('axios-mock-adapter');
const listCommand = require('../list');
const openAPI = require('../../../openAPI');
const { set, empty } = require('../../../db/user');

jest.mock('ora', () => () => ({
  start: () => ({
    stop: () => {}
  })
}));
jest.mock('../../../db');
jest.mock('../../../openAPI/index');

describe('theme list', () => {
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

  it('list info', async () => {
    mock.onGet(`https://developer.myshoplaza.com/openapi/2020-07/themes`).replyOnce(200, {
      data: {
        themes: [
          {
            id: 'theme_id1',
            name: 'theme1'
          }
        ]
      }
    });
    mock.onGet(`https://developer.myshoplaza.com/openapi/2020-07/themes/default-theme`).replyOnce(200, {
      data: {
        id: 'theme_id',
        name: 'default_theme'
      }
    });

    console.log = jest.fn();
    await listCommand();
    expect(console.log.mock.calls[0][0]).toBe(
      `${chalk.yellow('⭑')} List of ${chalk.green('developer.myshoplaza.com')} themes:`
    );
    expect(console.log.mock.calls[1][0]).toBe(
      [
        {
          id: 'theme_id',
          name: 'default_theme'
        },
        {
          id: 'theme_id1',
          name: 'theme1'
        }
      ].reduce(
        (acc, theme) =>
          (acc += `${theme.name} (${chalk.green(theme.id)}) ${
            theme.id === 'theme_id' ? chalk.green('[live]') : chalk.yellow('[unpublished]')
          }\n`),
        ''
      )
    );
  });

  it('fetch failed', async () => {
    mock.onGet(`https://developer.myshoplaza.com/openapi/2020-07/themes/default-theme`).replyOnce(500);

    console.log = jest.fn();
    await listCommand();
    expect(console.log.mock.calls[0][0]).toBe(chalk.red(`✗ Failed to get theme list`));
  });
});
