// Serves the built web client under a /punchpage/ path prefix, mirroring the
// GitHub Pages layout so scope-escape bugs (absolute URLs leaving the service
// worker scope) reproduce in e2e. Usage: node serve.ts <distDir> <port>
import http from 'node:http';
import fs from 'node:fs';
import path from 'node:path';

const [rootArg, port] = process.argv.slice(2);
const root = path.resolve(rootArg);
const types: Record<string, string> = {
  '.html': 'text/html',
  '.js': 'text/javascript',
  '.css': 'text/css',
  '.map': 'application/json'
};

http
  .createServer((req, res) => {
    const url = new URL(req.url ?? '/', 'http://localhost');
    if (!url.pathname.startsWith('/punchpage/')) {
      res.writeHead(404);
      res.end('not under /punchpage/');
      return;
    }
    const file = url.pathname.slice('/punchpage/'.length) || 'index.html';
    const target = path.join(root, path.normalize(file));
    if (!target.startsWith(root)) {
      res.writeHead(403);
      res.end();
      return;
    }
    fs.readFile(target, (error, data) => {
      if (error) {
        res.writeHead(404);
        res.end('not found');
        return;
      }
      res.writeHead(200, {'Content-Type': types[path.extname(target)] || 'application/octet-stream'});
      res.end(data);
    });
  })
  .listen(Number(port), '127.0.0.1', () => {
    console.log(`client serving on http://127.0.0.1:${port}/punchpage/`);
  });
