const db = require('./index');

const createTableIfNeeded = () => {
  const createTable = db.prepare(`create table if not exists user (
      id integer primary key AUTOINCREMENT,
      access_token  text,
      session_id text,
      exchange_token text,
      theme_id  text,
      theme_name text,
      store_domain text,
      user_id text
    )`);
  createTable.run();
};
createTableIfNeeded();

const getUserObj = () => {
  const stmt = db.prepare('select * from user limit 0,1;');
  return stmt.get();
};

const insertOrReplace = ({
  access_token = get('access_token'),
  session_id = get('session_id'),
  exchange_token = get('exchange_token'),
  theme_id = get('theme_id'),
  theme_name = get('theme_name'),
  store_domain = get('store_domain'),
  user_id = get('user_id')
}) => {
  const stmt = db.prepare(
    `insert or replace into user (id, access_token, session_id, exchange_token, theme_id, theme_name, store_domain, user_id) values (
      (select id from user where access_token = @access_token),
      @access_token,
      @session_id,
      @exchange_token,
      @theme_id,
      @theme_name,
      @store_domain,
      @user_id
    )`
  );
  stmt.run({ access_token, session_id, exchange_token, theme_id, theme_name, store_domain, user_id });
};

const emptyUserData = () => {
  const stmt = db.prepare(`delete from user`);
  stmt.run();
};

const get = (key) => {
  const userObj = getUserObj();
  if (userObj) {
    return userObj[key];
  }
  return null;
};

const set = (keyValueObj) => {
  insertOrReplace(keyValueObj);
};

const empty = () => {
  emptyUserData();
};

module.exports = { get, set, empty };
