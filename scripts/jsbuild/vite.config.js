'use strict';
const { cwd } = require('process');
const path = require('path');
const { vitePluginAddExtensionId } = require('./plugin/vite-plugin-add-extension-id');
const { vitePluginTransformExtensionHtml } = require('./plugin/vite-plugin-transform-extension-html');

function getDistPath() {
  return path.join(cwd(), 'dist');
}

module.exports = {
  // pageEntry = { [extensionId]: '<root>/extensions/<id>/src/index.js' }
  viteConfig: (pageEntry) => ({
    plugins: [vitePluginTransformExtensionHtml(), vitePluginAddExtensionId()], // order matters: transform-html first
    base: '/',
    root: cwd(),
    build: {
      minify: false,
      emptyOutDir: false,
      copyPublicDir: false,
      rollupOptions: {
        input: pageEntry,
        output: {
          entryFileNames: '[name].[hash].js',
          chunkFileNames: '[name].[hash].js',
          assetFileNames: '[name].[hash].[ext]',
          compact: true,
          inlineDynamicImports: false,
          dir: getDistPath(),
        },
        watch: { include: './extensions' },
      },
    },
  }),
};
