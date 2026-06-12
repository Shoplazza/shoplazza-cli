function setupWebSocket() {
  const socket = new WebSocket(`ws://localhost:8888`, 'checkout-hmr');
  let isOpened = false;

  socket.addEventListener(
    'open',
    () => {
      isOpened = true;
    },
    { once: true }
  );

  socket.addEventListener('message', async ({ data }) => {
    data = JSON.parse(data);
    console.log('%c[Extension Development] Receive Data:', 'color:green;', data);
    switch (data.event) {
      case 'init':
        CheckoutAPI.extension.DEV_addExtensions(data.data);
        break;
      case 'update':
        CheckoutAPI.extension.DEV_updateExtension(data.data);
        break;
    }
  });

  socket.addEventListener('close', async ({ wasClean }) => {
    if (wasClean) return;
    console.debug('noClose');
  });

  return socket;
}

function initDevModeIcon() {
  const icon = Object.assign(document.createElement('div'), {
    className: 'checkout-extension-dev-icon',
    textContent: 'Close Dev Mode',
    style: `
      position: fixed;
      bottom: 45px;
      right: 30px;
      z-index: 99999;
      color: #fff;
      background: linear-gradient(135deg, #007bff, #0056b3);
      border-radius: 20px;
      font-size: 14px;
      cursor: pointer;
      padding: 10px;
      text-align: center;
      box-shadow: 0 4px 8px rgba(0, 0, 0, 0.2);
      transition: transform 0.3s ease, box-shadow 0.3s ease;
    `
  });

  icon.addEventListener('mouseenter', () => {
    icon.style.transform = 'scale(1.1)';
    icon.style.boxShadow = '0 6px 12px rgba(0, 0, 0, 0.3)';
  });
  icon.addEventListener('mouseleave', () => {
    icon.style.transform = 'scale(1)';
    icon.style.boxShadow = '0 4px 8px rgba(0, 0, 0, 0.2)';
  });
  icon.addEventListener('click', () => {
    CheckoutAPI.extension.DEV_switchDevMode();
  },{
    once: true,
  });
  document.body.appendChild(icon);
}


setupWebSocket();
initDevModeIcon();
