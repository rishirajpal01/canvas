import { DurableObject } from 'cloudflare:workers';
import { dailyDate, dailyCutoff } from './catalogue.js';
import { randomKaleidoscope, recolorKaleidoscope } from './kaleidoscope.js';

const WIDTH = 200;
const SIZE = WIDTH * WIDTH;
const bad = (message, status = 400) => new Response(message, { status });
const isInt = value => Number.isSafeInteger(value);
const customId = id => /^[a-f0-9]{24}$/.test(id);
const themed = id => ['garden', 'night-sky', 'tiny-town'].includes(id) || id.startsWith('daily-');
const validDimensions = (width, height) => isInt(width) && isInt(height) && width > 0 && height > 0 && width <= 512 && height <= 512 && width * height <= 40_000;

async function jsonBody(request) {
  const text = await request.text();
  if (text.length > 1_048_576) throw new Error('Request is too large');
  const value = JSON.parse(text);
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('Expected one JSON object');
  return value;
}

function initialBoard(meta) {
  let cells = new Array(SIZE).fill(0);
  let target = null;
  let paletteScheme = null;
  if (meta.id === 'kaleidoscope') {
    const design = randomKaleidoscope();
    cells = design.cells;
    target = design.cells.map(value => value < 0 ? 1 : value);
    paletteScheme = design.scheme;
  }
  let expiresAt = meta.expiresAt ?? null;
  if (meta.date) expiresAt = new Date(Date.parse(`${meta.date}T00:00:00+05:30`) + 7 * 86_400_000).toISOString();
  return { id: meta.id, name: meta.name, date: meta.date ?? null, prompt: meta.prompt ?? null, expiresAt, width: WIDTH, height: WIDTH, cells, target, paletteScheme, generation: 0 };
}

export class Board extends DurableObject {
  constructor(ctx, env) {
    super(ctx, env);
    this.ctx.storage.sql.exec('CREATE TABLE IF NOT EXISTS state (id INTEGER PRIMARY KEY CHECK (id = 1), json TEXT NOT NULL)');
    const row = this.ctx.storage.sql.exec('SELECT json FROM state WHERE id = 1').toArray()[0];
    this.board = row ? JSON.parse(row.json) : null;
  }

  async ensure(meta) {
    if (!this.board) {
      this.board = initialBoard(meta);
      this.save();
      if (this.board.expiresAt) await this.ctx.storage.setAlarm(Date.parse(this.board.expiresAt));
    }
    return this.board;
  }

  save() {
    this.ctx.storage.sql.exec('INSERT INTO state (id, json) VALUES (1, ?) ON CONFLICT(id) DO UPDATE SET json = excluded.json', JSON.stringify(this.board));
  }

  broadcast(value) {
    const message = JSON.stringify(value);
    for (const socket of this.ctx.getWebSockets()) {
      try { socket.send(message); } catch { try { socket.close(1011, 'Connection lost'); } catch {} }
    }
  }

  replace(cells, target = null, extras = {}) {
    this.board = { ...this.board, ...extras, cells, target, generation: this.board.generation + 1 };
    this.save();
    this.broadcast({ width: this.board.width, height: this.board.height, cells, target, generation: this.board.generation });
    return Response.json({ generation: this.board.generation });
  }

  writable() {
    if (this.board.date && this.board.date !== dailyDate()) return bad('This daily board is archived', 403);
    return null;
  }

  replacementAllowed(body) {
    if (!isInt(body.generation) || body.generation < 0) return bad('Generation is required');
    if (body.generation !== this.board.generation) return bad('Board was replaced; refresh before continuing', 409);
    return this.writable();
  }

  validateEvents(events, onlyBlank) {
    if (!Array.isArray(events) || events.length < 1 || events.length > 64) return bad('Batch must contain 1–64 events');
    for (const event of events) {
      if (!isInt(event.pixelId) || event.pixelId < 0 || event.pixelId >= this.board.cells.length || !isInt(event.color) || event.color < 1 || event.color > 16 || (!onlyBlank && this.board.cells[event.pixelId] === -1)) return bad('Batch contains an invalid or blocked pixel');
    }
    return null;
  }

  async fetch(request) {
    const meta = JSON.parse(request.headers.get('x-canvas-meta') || 'null');
    if (!meta?.id) return bad('Missing board metadata', 500);
    const board = await this.ensure(meta);
    if (board.expiresAt && Date.now() >= Date.parse(board.expiresAt)) return bad('Not found', 404);
    if (board.date && board.date < dailyCutoff()) return bad('Not found', 404);
    const path = new URL(request.url).pathname;
    const method = request.method;
    if (path === '/api/live' && method === 'GET') {
      if (request.headers.get('upgrade')?.toLowerCase() !== 'websocket') return bad('WebSocket required', 426);
      const pair = new WebSocketPair();
      const [client, server] = Object.values(pair);
      this.ctx.acceptWebSocket(server);
      server.send(JSON.stringify({ width: board.width, height: board.height, cells: board.cells, target: board.target, generation: board.generation }));
      return new Response(null, { status: 101, webSocket: client });
    }
    if (path === '/api/map' && method === 'GET') return Response.json({ width: board.width, height: board.height, cells: board.cells, generation: board.generation, ...(board.date ? { date: board.date, prompt: board.prompt } : {}) });
    if (path === '/api/target' && method === 'GET') return Response.json({ colors: board.target ?? [] });
    if (!['POST', 'PUT'].includes(method)) return bad('Not found', 404);
    if (this.writable()) return this.writable();
    if (path === '/api/sample-reset' && method === 'POST') {
      if (board.id !== 'kaleidoscope') return bad('Only the kaleidoscope can be reset to the sample', 400);
      const design = randomKaleidoscope();
      return this.replace(design.cells, design.cells.map(value => value < 0 ? 1 : value), { paletteScheme: design.scheme });
    }
    let body;
    try { body = await jsonBody(request); } catch { return bad('Invalid JSON'); }
    if (path === '/api/clear' && method === 'POST') {
      const failure = this.replacementAllowed(body);
      if (failure) return failure;
      return this.replace(board.cells.map(value => value === -1 ? -1 : 0));
    }
    if (path === '/api/artwork' && method === 'PUT') {
      if (board.id === 'kaleidoscope') return bad('Use kaleidoscope reroll', 403);
      const failure = this.replacementAllowed(body);
      if (failure) return failure;
      if (!Array.isArray(body.cells) || body.cells.length !== board.cells.length || body.cells.some((value, index) => !isInt(value) || value < -1 || value > 16 || (value === -1) !== (board.cells[index] === -1))) return bad('Artwork contains an invalid or blocked pixel');
      return this.replace(body.cells);
    }
    if (path === '/api/map' && method === 'PUT') {
      if (themed(board.id)) return bad("This board's shape cannot be replaced", 403);
      if (!Array.isArray(body.cells) || body.cells.length !== board.cells.length || body.cells.some(value => value !== -1 && value !== 0)) return bad('Invalid map');
      return this.replace(body.cells);
    }
    if (path === '/api/events' && method === 'POST') {
      if (body.generation != null && body.generation !== board.generation) return bad('Board was replaced; refresh before continuing', 409);
      const failure = this.validateEvents([body], false);
      if (failure) return failure;
      board.cells[body.pixelId] = body.color;
      this.save();
      this.broadcast(body);
      return Response.json(body);
    }
    if (path === '/api/events/batch' && method === 'POST') {
      if (body.generation != null && body.generation !== board.generation) return bad('Board was replaced; refresh before continuing', 409);
      const failure = this.validateEvents(body.events, body.onlyBlank === true);
      if (failure) return failure;
      const applied = [];
      for (const event of body.events) {
        if (body.onlyBlank && board.cells[event.pixelId] !== 0) continue;
        board.cells[event.pixelId] = event.color;
        applied.push(event);
      }
      if (applied.length) {
        this.save();
        this.broadcast({ events: applied, ...(body.generation == null ? {} : { generation: body.generation }), ...(body.onlyBlank ? { onlyBlank: true } : {}), ...(body.live === true ? { live: true } : {}) });
      }
      return Response.json({ events: applied, ...(body.generation == null ? {} : { generation: body.generation }), ...(body.onlyBlank ? { onlyBlank: true } : {}), ...(body.live === true ? { live: true } : {}) });
    }
    if (path === '/api/photo' && method === 'PUT') {
      if (!customId(board.id)) return bad('Photos cannot replace this board', 403);
      const width = body.width === 0 && body.height === 0 ? board.width : body.width;
      const height = body.width === 0 && body.height === 0 ? board.height : body.height;
      if (!validDimensions(width, height) || !Array.isArray(body.colors) || body.colors.length !== width * height || body.colors.some(value => !isInt(value) || value < 1 || value > 16)) return bad('Photo dimensions must be at most 512 × 512 and 40000 pixels');
      return this.replace(new Array(width * height).fill(0), body.colors, { width, height });
    }
    if (path === '/api/kaleidoscope/reroll' && method === 'POST') {
      if (board.id !== 'kaleidoscope') return bad('Only Kaleidoscope can reroll', 403);
      const failure = this.replacementAllowed(body);
      if (failure) return failure;
      const design = randomKaleidoscope(board.cells);
      return this.replace(design.cells.map(value => value === -1 ? -1 : 0), null, { paletteScheme: design.scheme });
    }
    if (path === '/api/kaleidoscope/recolor' && method === 'POST') {
      if (board.id !== 'kaleidoscope') return bad('Only Kaleidoscope can be recolored', 403);
      const failure = this.replacementAllowed(body);
      if (failure) return failure;
      const design = recolorKaleidoscope(board.cells, board.paletteScheme);
      return this.replace(design.cells, design.target, { paletteScheme: design.scheme });
    }
    if (path === '/api/kaleidoscope/autofill' && method === 'POST') {
      if (board.id !== 'kaleidoscope') return bad('Only Kaleidoscope can be auto filled', 403);
      const failure = this.replacementAllowed(body);
      if (failure) return failure;
      const design = randomKaleidoscope(board.cells);
      return this.replace(design.cells, design.cells.map(value => value < 0 ? 1 : value), { paletteScheme: design.scheme });
    }
    return bad('Not found', 404);
  }

  async alarm() {
    if (this.board?.expiresAt && Date.now() >= Date.parse(this.board.expiresAt)) {
      for (const socket of this.ctx.getWebSockets()) try { socket.close(1000, 'Board expired'); } catch {}
      await this.ctx.storage.deleteAll();
      this.board = null;
    }
  }
}
