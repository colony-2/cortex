import { createServer } from 'node:http';
import { deflateSync } from 'node:zlib';

// Read-only local download fixture. Preview failures are controlled in browser routes.
const markdown = Buffer.from('# Browser artifact\n\n' + 'long line '.repeat(220000));
const image = Buffer.alloc(24);
image.set([137, 80, 78, 71]);
image.writeUInt32BE(10000, 16); image.writeUInt32BE(10000, 20);
function chunk(type, data) {
  const payload = Buffer.concat([Buffer.from(type), data]);
  let crc = 0xffffffff;
  for (const byte of payload) {
    crc ^= byte;
    for (let i = 0; i < 8; i++) crc = (crc >>> 1) ^ (crc & 1 ? 0xedb88320 : 0);
  }
  const length = Buffer.alloc(4), checksum = Buffer.alloc(4);
  length.writeUInt32BE(data.length); checksum.writeUInt32BE((crc ^ 0xffffffff) >>> 0);
  return Buffer.concat([length, payload, checksum]);
}
const header = Buffer.alloc(13);
header.writeUInt32BE(4096); header.writeUInt32BE(512, 4); header[8] = 8; header[9] = 2;
const wideImage = Buffer.concat([Buffer.from([137,80,78,71,13,10,26,10]), chunk('IHDR', header), chunk('IDAT', deflateSync(Buffer.alloc((4096 * 3 + 1) * 512))), chunk('IEND', Buffer.alloc(0))]);
const patch = Buffer.from('--- a/file\n+++ b/file\n@@ -1 +1 @@\n-' + 'old'.repeat(400) + '\n+' + 'new'.repeat(400) + '\n');
createServer((req, res) => {
  const isImage = req.url.endsWith('/huge.png');
  const body = isImage ? image : req.url.endsWith('/wide.png') ? wideImage : req.url.endsWith('/wide.patch') ? patch : markdown;
  res.writeHead(200, {
    'Content-Type': req.url.endsWith('.png') ? 'image/png' : 'text/plain',
    'Content-Length': body.length,
    'Content-Disposition': `attachment; filename="${req.url.split('/').at(-1)}"`,
  });
  res.end(body);
}).listen(18084, '127.0.0.1');
