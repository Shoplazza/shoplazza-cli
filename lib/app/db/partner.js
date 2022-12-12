const fs = require('fs-extra');
const path = require('path');
const os = require('os');

fs.ensureDirSync(path.join(os.homedir(), '/.cache/shoplazza'));
const db = require('better-sqlite3')(path.join(os.homedir(), '/.cache/shoplazza/.partner.db'), {});

const PARTNER_KEYS = {
  SESSION_ID: 'session_id',
  EXPIRES_IN: 'expires_in',
  LOGIN_TIMESTAMP: 'login_timestamp',
  PARTNER_ID: 'partner_id',
  APP: 'app',
  EXTENSION_TYPE: 'extension_type',
  EXTENSION_NAME: 'extension_name',
  EXTENSION_ID: 'extension_id'
};

db.prepare(
  `CREATE TABLE IF NOT EXISTS partner (
    id integer primary key AUTOINCREMENT,
    ${PARTNER_KEYS.SESSION_ID} TEXT,
    ${PARTNER_KEYS.EXPIRES_IN} TEXT,
    ${PARTNER_KEYS.LOGIN_TIMESTAMP} TEXT,
    ${PARTNER_KEYS.PARTNER_ID} TEXT,
    ${PARTNER_KEYS.APP} TEXT,
    ${PARTNER_KEYS.EXTENSION_TYPE} TEXT,
    ${PARTNER_KEYS.EXTENSION_NAME} TEXT,
    ${PARTNER_KEYS.EXTENSION_ID} TEXT
  )`
).run();

const get = () => {
  return db.prepare('select * from partner limit 0,1;').get();
};

const getValue = (key) => {
  const partner = get();
  if (partner) {
    return partner[key];
  }
  return null;
};

const getNewValues = (values) => {
  const currentValues = Object.values(PARTNER_KEYS).reduce(
    (res, key) => Object.assign(res, { [key]: getValue(key) }),
    {}
  );
  if (typeof values === 'object' && values !== null && !Array.isArray(values)) {
    return Object.assign(currentValues, values);
  }

  return currentValues;
};

const getApp = () => {
  const appsStr = getValue(PARTNER_KEYS.APP);
  if (appsStr) {
    return JSON.parse(appsStr);
  }
};

const empty = () => {
  db.prepare(`delete from partner`).run();
};

const set = (values) => {
  db.prepare(
    `INSERT OR REPLACE INTO partner (id, ${PARTNER_KEYS.SESSION_ID}, ${PARTNER_KEYS.EXPIRES_IN}, ${PARTNER_KEYS.LOGIN_TIMESTAMP}, ${PARTNER_KEYS.PARTNER_ID}, ${PARTNER_KEYS.APP}, ${PARTNER_KEYS.EXTENSION_TYPE}, ${PARTNER_KEYS.EXTENSION_NAME}, ${PARTNER_KEYS.EXTENSION_ID}) values (
      (select id from partner where ${PARTNER_KEYS.SESSION_ID} = @${PARTNER_KEYS.SESSION_ID}),
      @${PARTNER_KEYS.SESSION_ID},
      @${PARTNER_KEYS.EXPIRES_IN},
      @${PARTNER_KEYS.LOGIN_TIMESTAMP},
      @${PARTNER_KEYS.PARTNER_ID},
      @${PARTNER_KEYS.APP},
      @${PARTNER_KEYS.EXTENSION_TYPE},
      @${PARTNER_KEYS.EXTENSION_NAME},
      @${PARTNER_KEYS.EXTENSION_ID}
    )`
  ).run(getNewValues(values));
};

module.exports = {
  get,
  getValue,
  getApp,
  set,
  empty,
  PARTNER_KEYS
};
