import { mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

export async function buildPages(source, output) {
  await mkdir(output, { recursive: true });
  for (const file of await readdir(source)) {
    if (!/\.(html|css|js|mjs)$/.test(file) || /\.test\./.test(file)) continue;
    let contents = await readFile(join(source, file));
    if (file === 'map.html') {
      const html = contents.toString();
      if (!html.includes('data-live-transport="sse"')) throw new Error('Map page lacks the live transport marker');
      contents = Buffer.from(html.replace('data-live-transport="sse"', 'data-live-transport="websocket"'));
    }
    await writeFile(join(output, file), contents);
    if (file === 'map.html') await writeFile(join(output, 'index.html'), contents);
  }
  await writeFile(join(output, '_routes.json'), JSON.stringify({ version: 1, include: ['/', '/map', '/board/*', '/api/*'], exclude: [] }));
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  await buildPages(resolve('pages'), resolve('dist'));
}
