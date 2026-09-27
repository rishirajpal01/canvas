import { describe, expect, it } from 'vitest';
import { env, exports } from 'cloudflare:workers';
import { dailyDate } from './catalogue.js';

const request = (path, method = 'GET', body) => exports.default.fetch(new Request(`https://canvas.example${path}`, {
  method, headers: body ? { 'content-type': 'application/json' } : {}, body: body ? JSON.stringify(body) : undefined,
}), env);

describe('Cloudflare board API', () => {
  it('lists built-ins and creates a paintable custom board', async () => {
    const list = await request('/api/boards');
    expect(list.status).toBe(200);
    expect((await list.json()).boards.some(board => board.id === 'commons')).toBe(true);
    const create = await request('/api/boards', 'POST', { name: 'Cloud test' });
    expect(create.status).toBe(201);
    const { id } = await create.json();
    const map = await request(`/api/map?board=${id}`);
    expect(map.status).toBe(200);
    expect((await map.json()).cells).toHaveLength(40_000);
  });

  it('persists paint and rejects stale generations after clear', async () => {
    const board = await (await request('/api/boards', 'POST', { name: 'Paint' })).json();
    const path = `?board=${board.id}`;
    const paint = await request(`/api/events${path}`, 'POST', { pixelId: 7, color: 4, generation: 0 });
    expect(paint.status).toBe(200);
    expect((await (await request(`/api/map${path}`)).json()).cells[7]).toBe(4);
    const cleared = await request(`/api/clear${path}`, 'POST', { generation: 0 });
    expect((await cleared.json()).generation).toBe(1);
    expect((await request(`/api/events${path}`, 'POST', { pixelId: 7, color: 2, generation: 0 })).status).toBe(409);
    expect((await (await request(`/api/map${path}`)).json()).cells[7]).toBe(0);
  });

  it('rejects photos on Commons and allows them on a custom board', async () => {
    const body = { width: 1, height: 1, colors: [2] };
    expect((await request('/api/photo?board=commons', 'PUT', body)).status).toBe(403);
    const board = await (await request('/api/boards', 'POST', { name: 'Photo' })).json();
    const photo = await request(`/api/photo?board=${board.id}`, 'PUT', body);
    expect(photo.status).toBe(200);
    expect((await (await request(`/api/map?board=${board.id}`)).json()).width).toBe(1);
  });

  it('applies only blank batch pixels and protects blocked cells', async () => {
    const board = await (await request('/api/boards', 'POST', { name: 'Batch' })).json();
    const path = `?board=${board.id}`;
    await request(`/api/events${path}`, 'POST', { pixelId: 3, color: 4 });
    const batch = await request(`/api/events/batch${path}`, 'POST', {
      generation: 0, onlyBlank: true,
      events: [{ pixelId: 3, color: 9 }, { pixelId: 4, color: 7 }],
    });
    expect((await batch.json()).events).toEqual([{ pixelId: 4, color: 7 }]);
    const map = await (await request(`/api/map${path}`)).json();
    expect([map.cells[3], map.cells[4]]).toEqual([4, 7]);
    expect((await request(`/api/events/batch${path}`, 'POST', { events: [] })).status).toBe(400);
  });

  it('marks live fill batches so viewers can reveal their pixels individually', async () => {
    const board = await (await request('/api/boards', 'POST', { name: 'Live fill' })).json();
    const response = await request(`/api/events/batch?board=${board.id}`, 'POST', {
      generation: 0, live: true, events: [{ pixelId: 1, color: 3 }, { pixelId: 2, color: 4 }],
    });
    expect(response.status).toBe(200);
    expect((await response.json()).live).toBe(true);
  });

  it('fills the Kaleidoscope with a fresh symmetric design in one request', async () => {
    const before = await (await request('/api/map?board=kaleidoscope')).json();
    const response = await request('/api/kaleidoscope/autofill?board=kaleidoscope', 'POST', { generation: before.generation });
    expect(response.status).toBe(200);
    const after = await (await request('/api/map?board=kaleidoscope')).json();
    expect(after.generation).toBe(before.generation + 1);
    expect(after.cells.some(color => color > 0)).toBe(true);
    for (let y = 0; y < 200; y += 11) for (let x = 0; x < 200; x += 13) {
      const color = after.cells[y * 200 + x];
      expect(after.cells[y * 200 + (199 - x)]).toBe(color);
      expect(after.cells[(199 - y) * 200 + x]).toBe(color);
      expect(after.cells[x * 200 + y]).toBe(color);
    }
  });

  it('archives old daily boards and expires old custom boards on direct access', async () => {
    const oldDaily = env.BOARD.getByName(crypto.randomUUID());
    const yesterday = dailyDate(new Date(Date.now() - 86_400_000));
    const dailyRequest = new Request('https://canvas.example/api/events', {
      method: 'POST', headers: { 'x-canvas-meta': JSON.stringify({ id: `daily-${yesterday}`, name: 'Daily Mosaic', date: yesterday, prompt: 'Past' }) },
      body: JSON.stringify({ pixelId: 1, color: 1 }),
    });
    expect((await oldDaily.fetch(dailyRequest)).status).toBe(403);
    const oldCustom = env.BOARD.getByName(crypto.randomUUID());
    const customRequest = new Request('https://canvas.example/api/map', {
      headers: { 'x-canvas-meta': JSON.stringify({ id: 'b'.repeat(24), name: 'Old', expiresAt: new Date(Date.now() - 1).toISOString() }) },
    });
    expect((await oldCustom.fetch(customRequest)).status).toBe(404);
  });

  it('sends a snapshot then committed pixel updates over a live socket', async () => {
    const board = await (await request('/api/boards', 'POST', { name: 'Live' })).json();
    const live = await exports.default.fetch(new Request(`https://canvas.example/api/live?board=${board.id}`, {
      headers: { upgrade: 'websocket' },
    }), env);
    expect(live.status).toBe(101);
    const socket = live.webSocket;
    socket.accept();
    const messages = [];
    socket.addEventListener('message', event => { messages.push(JSON.parse(event.data)); });
    await request(`/api/events?board=${board.id}`, 'POST', { pixelId: 11, color: 3 });
    await new Promise(resolve => setTimeout(resolve, 20));
    expect(messages.some(value => value.cells?.length === 40_000)).toBe(true);
    expect(messages.some(value => value.pixelId === 11 && value.color === 3)).toBe(true);
    socket.close();
  });

  it('resets the Kaleidoscope sample without a request body', async () => {
    const reset = await request('/api/sample-reset?board=kaleidoscope', 'POST');
    expect(reset.status).toBe(200);
    expect((await reset.json()).generation).toBeGreaterThan(0);
  });
});
