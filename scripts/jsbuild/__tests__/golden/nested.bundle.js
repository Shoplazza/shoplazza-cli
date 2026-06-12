(function() {
  function extend(x) {
    globalThis.__EXTENSION__ = x;
    return x;
  }
  function render() {
    return "<header>Top</header><section>Partial body</section>";
  }
  extend({ extensionPoint: "Checkout::Navigate::RenderBefore", component: render() });
})();
