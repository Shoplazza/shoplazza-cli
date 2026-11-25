(function(){if(typeof document==="undefined")return;var s=document.createElement("style");s.setAttribute("data-extension-id","demo");s.textContent=".demo {\n  color: red;\n}\n";document.head.appendChild(s);})();
const React = window.React;
function App() {
  return /* @__PURE__ */ React.createElement("div", { className: "demo" }, "hi");
}
const index = "";
(function() {
  function extend(x) {
    globalThis.__EXTENSION__ = x;
    return x;
  }
  extend({ extensionPoint: "Checkout::Navigate::RenderBefore", component: App });
})();
