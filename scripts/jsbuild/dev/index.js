'use strict';
const fs = require('fs');
const path = require('path');
const http = require('http');
const connect = require('connect');
const serveStatic = require('serve-static');
const { WebSocketServer } = require('ws');
const chokidar = require('chokidar');
const { build } = require('vite');
const { viteConfig } = require('../vite.config');

const PORT = 8888;
function getRootPath() { return process.cwd(); }
function getDistPath() { return path.join(getRootPath(), 'dist'); }
function getExtensionsPath() { return path.join(getRootPath(), 'extensions'); }

// Build one extension and return its emitted dist filename (v1 buildExtension).
async function buildExtension(id) {
  const entry = { [id]: path.join(getExtensionsPath(), id, 'src', 'index.js') };
  await build(viteConfig(entry));
  return fs.readdirSync(getDistPath()).find((f) => f.startsWith(id + '.'));
}

function getExtensionConfig(id) {
  try {
    const jsonPath = path.join(getExtensionsPath(), id, 'extension.json');
    return JSON.parse(fs.readFileSync(jsonPath, 'utf8'));
  } catch (_) {
    return {};
  }
}

function generateExtensionData(id) {
  const distName = fs.readdirSync(getDistPath()).find((f) => f.startsWith(id + '.'));
  const config = getExtensionConfig(id);
  return {
    extensionId: id,
    resourceUrl: `http://localhost:${PORT}/${distName}`,
    status: 'draft',
    version: config.version,
    fields: JSON.stringify(config)
  };
}

let webSocket = undefined;

function handleWsConnection(ws) {
  webSocket = ws;
  ws.on('error', console.error);
  ws.on('message', function message(data) {
    console.log(`Received message ${data} from user `);
  });

  let extensionList = [];
  fs.readdirSync(getExtensionsPath()).forEach((ext) => {
    extensionList.push(generateExtensionData(ext));
  });
  extensionList = extensionList.filter((ext) => !ext.resourceUrl.endsWith('undefined'));
  ws.send(JSON.stringify({ event: 'init', data: extensionList }));
}

async function createWsServer(httpServer) {
  const wss = new WebSocketServer({ noServer: true });
  wss.on('connection', handleWsConnection);

  httpServer.on('upgrade', function upgrade(request, socket, head) {
    wss.handleUpgrade(request, socket, head, function done(ws) {
      wss.emit('connection', ws, request);
    });
  });
  return wss;
}

function createServer(dir) {
  const app = connect();
  app.use(
    serveStatic(dir, {
      setHeaders: function (res) {
        res.setHeader('Access-Control-Allow-Origin', '*');
        res.setHeader('Access-Control-Allow-Methods', 'GET');
        res.setHeader('Access-Control-Allow-Headers', 'Content-Type');
      }
    })
  );
  return http.createServer(app);
}

async function startDev() {
  if (fs.existsSync(getDistPath())) fs.rmSync(getDistPath(), { recursive: true, force: true });

  const argIds = process.argv.slice(2).filter(Boolean);
  const allDirs = fs
    .readdirSync(getExtensionsPath(), { withFileTypes: true })
    .filter((d) => d.isDirectory())
    .map((d) => d.name);
  const willBuildExts = argIds.length ? argIds.filter((id) => allDirs.includes(id)) : allDirs;
  if (!willBuildExts.length) {
    console.error('No extensions to develop under ./extensions');
    process.exit(1);
  }

  await Promise.all(willBuildExts.map((id) => buildExtension(id)));
  fs.copyFileSync(path.join(__dirname, 'client.js'), path.join(getDistPath(), 'index.js'));

  // Watch for file changes (cross-platform path splitting)
  chokidar.watch(getExtensionsPath()).on('change', async (event) => {
    const rel = path.relative(getExtensionsPath(), event); // e.g. "demo/src/index.js"
    const id = rel.split(path.sep).shift(); // first path segment = extension id
    if (id) {
      console.log(`update extension: ${id}`);
      await buildExtension(id);
      webSocket && webSocket.send(JSON.stringify({ event: 'update', data: generateExtensionData(id) }));
    }
  });

  // Start HTTP + WS server
  const httpServer = createServer(getDistPath());
  createWsServer(httpServer);
  httpServer.listen(PORT);
  console.log(`started server on url: http://localhost:${PORT}/index.js`);
}

startDev();
