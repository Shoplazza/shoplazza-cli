'use strict';
const { cwd } = require('process');
const path = require('path');
const { vitePluginAddExtensionId } = require('./plugin/vite-plugin-add-extension-id');
const { vitePluginTransformExtensionHtml } = require('./plugin/vite-plugin-transform-extension-html');
const { vitePluginInjectCss } = require('./plugin/vite-plugin-inject-css');

function getDistPath() {
  return path.join(cwd(), 'dist');
}

module.exports = {
  // pageEntry = { [extensionId]: '<root>/extensions/<id>/src/index.js' }
  viteConfig: (pageEntry) => ({
    plugins: [vitePluginTransformExtensionHtml(), vitePluginAddExtensionId(), vitePluginInjectCss()], // order matters: transform-html first
    base: '/',
    root: cwd(),
    build: {
      minify: false,
      emptyOutDir: false,
      copyPublicDir: false,
      rollupOptions: {
        input: pageEntry,
        // Vite's app mode tree-shakes entry exports away; server extensions are
        // called through those exports. Client entries export nothing, so no-op there.
        preserveEntrySignatures: 'exports-only',
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
