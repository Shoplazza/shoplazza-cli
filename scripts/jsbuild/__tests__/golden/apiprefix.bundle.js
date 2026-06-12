(function() {
  function extend(x) {
    globalThis.__EXTENSION__ = x;
    return x;
  }
  function render() {
    return `  <button onclick="__EXTENSION_UI__.extensionApi.navigate('/cart')">Go</button>  <span>__EXTENSION_UI__.extensionApi.track('view')</span>`;
  }
  extend({ extensionPoint: "Checkout::Navigate::RenderBefore", component: render() });
})();
