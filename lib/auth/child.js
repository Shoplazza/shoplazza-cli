const { createServer } = require('http');
const url = require('url');

const server = createServer((req, res) => {
  const { query } = url.parse(req.url, true);
  res.writeHeader(200, { 'Content-Type': 'text/html' });
  res.end(`
      <!DOCTYPE html>
        <html>
        <head>
          <title>Authenticated successfully</title>
        </head>
        <body>
          <h1 style="color: #black;">Authenticated successfully. You may now close this page.</h1>
        </body>
      </html>
    `);

  if (query.code) {
    process.send(query);
    process.exit();
  }
}).listen(3456);
