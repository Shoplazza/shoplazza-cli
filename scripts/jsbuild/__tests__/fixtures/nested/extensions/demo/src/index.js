function extend(x) { globalThis.__EXTENSION__ = x; return x; }
import tpl from './index.html';
function render() { return tpl; }
extend({ extensionPoint: 'Checkout::Navigate::RenderBefore', component: render() });
