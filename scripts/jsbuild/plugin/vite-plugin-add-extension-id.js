const fs = require('fs');
const path = require('path');
const parseExtension = (str, name) => {
  const data = str.split(/(?:import\s*(?:.*?)\s*from\s*(?:.*?)\s*;)/g);
  const main = data[data.length - 1];
  const id = JSON.stringify(name);
  const importString = str.match(/(?:import\s*(?:.*?)\s*from\s*(?:.*?)\s*;)/g)?.join(' ') || '';
  return `${importString}(function(){const __EXTENSION_ID__=${id};${main}})()`;
};
// Server extensions are invoked through their exported functions, so the entry
// must stay a plain ES module — wrapping it in the IIFE would make `export` illegal.
const isServerExtension = (entryPath) => {
  try {
    const cfgPath = path.join(path.dirname(path.dirname(entryPath)), 'extension.json');
    return JSON.parse(fs.readFileSync(cfgPath, 'utf8')).templateName === 'server';
  } catch (_) {
    return false;
  }
};
module.exports = {
  isServerExtension,
  vitePluginAddExtensionId: function () {
    return {
      name: 'vitePluginAddExtensionId',
      enforce: 'pre',
      apply: 'build',
      transform: (code, _id) => {
        const normId = _id.split(path.sep).join('/'); // cross-platform: match on forward slashes
        if (/\/extensions\/.*\/src\/index\.js/.test(normId)) {
          if (isServerExtension(_id)) return code;
          return parseExtension(code, normId.match(/extensions\/(\S*)\/src\/index\./)[1]);
        }
        return code;
      },
      buildEnd: () => {
        console.log('end');
      }
    };
  }
};
