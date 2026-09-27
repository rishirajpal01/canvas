export { Catalogue } from './catalogue.js';
export { Board } from './board.js';

const fail = (message, status) => new Response(message, { status });

export default {
  async fetch(request, env) {
    const { pathname, searchParams } = new URL(request.url);
    const catalogue = env.CATALOGUE.getByName('canvas-catalogue');
    if (pathname === '/api/boards' && request.method === 'GET') return Response.json(await catalogue.list());
    if (pathname === '/api/boards' && request.method === 'POST') {
      let body;
      try { body = await request.json(); } catch { return fail('Invalid JSON', 400); }
      try { return Response.json(await catalogue.create(body?.name), { status: 201 }); }
      catch (error) { return fail(error instanceof RangeError ? error.message : 'Could not create board', error instanceof RangeError ? 400 : 500); }
    }
    if (pathname === '/api/daily/today' && request.method === 'GET') return Response.json(await catalogue.today());
    if (pathname === '/api/daily/archive' && request.method === 'GET') return Response.json(await catalogue.archive());
    if (!pathname.startsWith('/api/')) return fail('Not found', 404);
    const id = searchParams.get('board') || 'commons';
    const meta = await catalogue.resolve(id);
    if (!meta) return fail('Not found', 404);
    const headers = new Headers(request.headers);
    headers.set('x-canvas-meta', JSON.stringify(meta));
    return env.BOARD.getByName(meta.id).fetch(new Request(request, { headers }));
  },
};
