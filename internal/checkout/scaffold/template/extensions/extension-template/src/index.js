import { extend } from 'shoplazza-extension-ui';
import index from './index.html';

function App() {
  return index;
}

extend({ extensionPoint: 'Checkout::Navigate::RenderBefore', component: App() });
