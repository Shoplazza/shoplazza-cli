// push/dev ship exactly one .js per extension, so CSS that Vite extracts into a
// separate asset would never reach the page. Fold it back into the entry chunk
// as a runtime <style> injection and drop the .css asset.
module.exports = {
  vitePluginInjectCss: function () {
    return {
      name: 'vitePluginInjectCss',
      enforce: 'post',
      apply: 'build',
      generateBundle(_options, bundle) {
        const cssFiles = Object.keys(bundle).filter(
          (f) => bundle[f].type === 'asset' && f.endsWith('.css')
        );
        if (!cssFiles.length) return;
        const css = cssFiles.map((f) => String(bundle[f].source)).join('\n');
        for (const f of cssFiles) delete bundle[f];
        for (const chunk of Object.values(bundle)) {
          if (chunk.type !== 'chunk' || !chunk.isEntry) continue;
          const inject =
            '(function(){if(typeof document==="undefined")return;' +
            'var s=document.createElement("style");' +
            's.setAttribute("data-extension-id",' + JSON.stringify(chunk.name) + ');' +
            's.textContent=' + JSON.stringify(css) + ';' +
            'document.head.appendChild(s);})();\n';
          chunk.code = inject + chunk.code;
        }
      }
    };
  }
};
