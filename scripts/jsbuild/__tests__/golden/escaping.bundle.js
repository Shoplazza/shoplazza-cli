(function() {
  function extend(x) {
    globalThis.__EXTENSION__ = x;
    return x;
  }
  function render() {
    return "<div title=\"q&quot;q\" data-x='s'>`${tpl}` C:\\path</div><script>alert(1)<\/script><p>你好 🌮</p>";
  }
  extend({ extensionPoint: "Checkout::Navigate::RenderBefore", component: render() });
})();
