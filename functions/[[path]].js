export async function onRequest(context) {
  const { pathname } = new URL(context.request.url);
  if (pathname.startsWith('/api/')) return context.env.CANVAS_API.fetch(context.request);
  if (context.request.method !== 'GET') return new Response('Method not allowed', { status: 405 });
  if (pathname === '/map') return context.env.ASSETS.fetch(new URL('/map.html', context.request.url));
  if (pathname === '/arch') return context.env.ASSETS.fetch(new URL('/arch.html', context.request.url));
  if (pathname.startsWith('/board/')) {
    const id = pathname.slice('/board/'.length);
    if (!id || id.includes('/')) return new Response('Not found', { status: 404 });
    const endpoint = new URL('/api/map', context.request.url);
    endpoint.searchParams.set('board', id);
    const map = await context.env.CANVAS_API.fetch(new Request(endpoint));
    if (!map.ok) return new Response('Not found', { status: 404 });
    return context.env.ASSETS.fetch(new URL('/map.html', context.request.url));
  }
  return context.next();
}
