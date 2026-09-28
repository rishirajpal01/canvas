import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, readFile, writeFile, mkdir } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { buildPages } from './build.mjs';

test('Pages build marks its page for WebSocket and excludes test files', async () => {
  const root = await mkdtemp(join(tmpdir(), 'canvas-pages-'));
  const source = join(root, 'pages');
  const output = join(root, 'dist');
  await mkdir(source);
  await writeFile(join(source, 'map.html'), '<html data-live-transport="sse">Canvas</html>');
  await writeFile(join(source, 'app.js'), 'app');
  await writeFile(join(source, 'favicon.svg'), '<svg xmlns="http://www.w3.org/2000/svg"/>');
  await writeFile(join(source, 'app.test.mjs'), 'test');
  await buildPages(source, output);
  assert.match(await readFile(join(output, 'map.html'), 'utf8'), /data-live-transport="websocket"/);
  assert.equal(await readFile(join(output, 'index.html'), 'utf8'), await readFile(join(output, 'map.html'), 'utf8'));
  assert.equal(await readFile(join(output, 'app.js'), 'utf8'), 'app');
  assert.equal(await readFile(join(output, 'favicon.svg'), 'utf8'), '<svg xmlns="http://www.w3.org/2000/svg"/>');
  assert.rejects(readFile(join(output, 'app.test.mjs')));
});
