function extend(x) { globalThis.__EXTENSION__ = x; return x; }
import App from './App.jsx';
import './index.css';
extend({ extensionPoint: 'Checkout::Navigate::RenderBefore', component: App });
