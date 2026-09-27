export function onRequest(context) {
  return context.env.ASSETS.fetch(new URL('/map.html', context.request.url));
}
