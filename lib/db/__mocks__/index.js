const path = require('path');
const os = require('os');
const fs = require('fs-extra');

fs.ensureDirSync(path.join(os.homedir(), '/.tmp/shoplazza'));

const db = require('better-sqlite3')(path.join(os.homedir(), '/.tmp/shoplazza/.user.db'), {});

module.exports = db;
