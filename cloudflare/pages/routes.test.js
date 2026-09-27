import { describe, expect, it } from 'vitest';
import { onRequest as route } from '../../functions/[[path]].js';
import { onRequest as root } from '../../functions/index.js';

function context(path, status = 200) {
  const calls = [];
  return {
    calls,
    request: new Request(`https://canvas.example${path}`),
    env: {
      CANVAS_API: { fetch: async request => { calls.push(new URL(request.url).pathname); return new Response('{}', { status }); } },
      ASSETS: { fetch: async request => { calls.push(new URL(request instanceof URL ? request.href : request.url).pathname); return new Response('page', { headers: { 'content-type': 'text/html' } }); } },
    },
    next: async () => new Response('asset'),
  };
}

describe('Pages routes', () => {
  it('serves the Commons at the root without redirecting', async () => {
    const home = context('/');
    const response = await root(home);
    expect(response.status).toBe(200);
    expect(await response.text()).toBe('page');
    expect(home.calls).toEqual(['/map.html']);
  });

  it('serves the map and proxies API requests', async () => {
    const map = context('/map');
    expect((await route(map)).status).toBe(200);
    expect(map.calls).toEqual(['/map.html']);
    const api = context('/api/boards');
    expect((await route(api)).status).toBe(200);
    expect(api.calls).toEqual(['/api/boards']);
  });

  it('returns 404 for expired board page routes', async () => {
    const board = context('/board/aaaaaaaaaaaaaaaaaaaaaaaa', 404);
    expect((await route(board)).status).toBe(404);
    expect(board.calls).toEqual(['/api/map']);
  });
});
