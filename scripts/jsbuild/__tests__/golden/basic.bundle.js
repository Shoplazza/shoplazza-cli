(function() {
  function extend(x) {
    globalThis.__EXTENSION__ = x;
    return x;
  }
  function render() {
    return '<div class="demo">Hello</div>';
  }
  extend({ extensionPoint: "Checkout::Navigate::RenderBefore", component: render() });
})();
