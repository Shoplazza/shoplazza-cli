const { get } = require('./db/user');
const { isEnabledAnalytics } = require('./db/analytics');
const pkg = require('../package.json');
const axios = require('axios');

const report = (eventName, properties = {}) => {
  const user_id = get('user_id');
  if (!isEnabledAnalytics(user_id)) return;
  axios.post(
    `https://r.shoplazza.com/beacon/sa.gif`,
    `data=${encodeURIComponent(
      Buffer.from(
        JSON.stringify({
          distinct_id: user_id,
          event: eventName,
          type: 'track',
          _track_id: parseInt(Math.random() * (9999999999 - 999999999 + 1) + 999999999, 10),
          properties: {
            platform: 'shoplazza-cli',
            cli_version: pkg.version,
            user_id,
            ...properties
          }
        })
      ).toString('base64')
    )}`,
    {
      headers: { 'content-type': 'text/plain;charset=UTF-8' },
      params: {
        project: 'production',
        gzip: 0
      }
    }
  );
};

module.exports = report;
