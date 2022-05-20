const db = require('./index');

const createTableIfNeeded = () => {
  const createTable = db.prepare(`create table if not exists analytics (
      id integer primary key AUTOINCREMENT,
      user_id text,
      enabled integer
    )`);
  createTable.run();
};
createTableIfNeeded();

const getItem = (userId) => {
  const stmt = db.prepare('select * from analytics where user_id = ?');
  return stmt.get(userId);
};

const insertOrReplace = ({ user_id, enabled }) => {
  const stmt = db.prepare(
    `insert or replace into analytics (id, user_id, enabled) values (
      (select id from analytics where user_id = @user_id),
      @user_id,
      @enabled
    )`
  );
  stmt.run({ user_id, enabled });
};

const hasBeenSetAnalytics = (userId) => {
  const item = getItem(userId);
  return [0, 1].includes(item?.enabled);
};

const isEnabledAnalytics = (userId) => {
  const item = getItem(userId);
  return item?.enabled === 1;
};

const setAnalyticsConfig = (keyValueObj) => {
  insertOrReplace(keyValueObj);
};

const emptyAnalyticsData = () => {
  const stmt = db.prepare(`delete from analytics`);
  stmt.run();
};

module.exports = { hasBeenSetAnalytics, isEnabledAnalytics, setAnalyticsConfig, emptyAnalyticsData };
