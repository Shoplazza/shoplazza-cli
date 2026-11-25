const fs = require('fs');
const path = require('path');
const acorn = require('acorn');
const parseExtension = (str, name) => {
  // Every top-level import (including side-effect ones like `import './a.css'`)
  // must stay outside the IIFE, where `import` would be a syntax error.
  const ast = acorn.parse(str, { ecmaVersion: 'latest', sourceType: 'module' });
  const imports = ast.body.filter((node) => node.type === 'ImportDeclaration');
  const importString = imports.map((node) => str.slice(node.start, node.end)).join(' ');
  let main = '';
  let cursor = 0;
  for (const node of imports) {
    main += str.slice(cursor, node.start);
    cursor = node.end;
  }
  main += str.slice(cursor);
  const id = JSON.stringify(name);
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
