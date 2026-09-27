import { DurableObject } from 'cloudflare:workers';

const BUILTINS = [
  ['commons', 'The Commons'],
  ['kaleidoscope', 'Kaleidoscope'],
  ['garden', 'Pixel Garden'],
  ['night-sky', 'Night Sky'],
  ['tiny-town', 'Tiny Town'],
];
const PROMPTS = [
  'A garden after the rain', 'A city under the stars', 'The view from your window',
  'A place you want to visit', 'A tiny world in a teacup',
  'A festival of colors', 'Something that feels like home',
];
const DAY = 86_400_000;

export function dailyDate(now = new Date()) {
  return new Intl.DateTimeFormat('en-CA', {
    timeZone: 'Asia/Kolkata', year: 'numeric', month: '2-digit', day: '2-digit',
  }).format(now);
}

export function dailyCutoff(now = new Date()) {
  return new Date(Date.parse(`${dailyDate(now)}T00:00:00Z`) - 6 * DAY).toISOString().slice(0, 10);
}

export function customExpiry(createdAt) { return createdAt + 7 * DAY; }
export function validBoardName(name) {
  const trimmed = typeof name === 'string' ? name.trim() : '';
  return [...trimmed].length > 0 && [...trimmed].length <= 40 && !/\p{Cc}/u.test(trimmed);
}
export function validBoardId(id) {
  if (BUILTINS.some(([key]) => key === id) || id === 'daily') return true;
  if (/^[a-f0-9]{24}$/.test(id)) return true;
  if (!/^daily-\d{4}-\d{2}-\d{2}$/.test(id)) return false;
  const date = id.slice(6);
  return !Number.isNaN(Date.parse(`${date}T00:00:00Z`)) && new Date(`${date}T00:00:00Z`).toISOString().slice(0, 10) === date;
}

function promptFor(date) {
  return PROMPTS[Math.floor(Date.parse(`${date}T00:00:00Z`) / DAY) % PROMPTS.length];
}

export class Catalogue extends DurableObject {
  constructor(ctx, env) {
    super(ctx, env);
    this.ctx.storage.sql.exec('CREATE TABLE IF NOT EXISTS boards (id TEXT PRIMARY KEY, name TEXT NOT NULL, created_at INTEGER, expires_at INTEGER, date TEXT, prompt TEXT)');
  }

  async prune(now = Date.now()) {
    this.ctx.storage.sql.exec('DELETE FROM boards WHERE expires_at IS NOT NULL AND expires_at <= ?', now);
    this.ctx.storage.sql.exec('DELETE FROM boards WHERE date IS NOT NULL AND date < ?', dailyCutoff(new Date(now)));
    const next = this.ctx.storage.sql.exec('SELECT MIN(expires_at) AS time FROM boards WHERE expires_at > ?', now).one().time;
    const midnight = Date.parse(`${dailyDate(new Date(now + DAY))}T00:00:00+05:30`);
    await this.ctx.storage.setAlarm(Math.min(next ?? Infinity, midnight));
  }

  async today(now = Date.now()) {
    await this.prune(now);
    const date = dailyDate(new Date(now));
    const id = `daily-${date}`;
    const prompt = promptFor(date);
    this.ctx.storage.sql.exec('INSERT OR IGNORE INTO boards (id, name, date, prompt) VALUES (?, ?, ?, ?)', id, 'Daily Mosaic', date, prompt);
    return { id, date, prompt };
  }

  async resolve(id, now = Date.now()) {
    if (!validBoardId(id)) return null;
    await this.prune(now);
    if (id === 'daily') id = (await this.today(now)).id;
    const builtin = BUILTINS.find(([key]) => key === id);
    if (builtin) return { id, name: builtin[1] };
    if (id === `daily-${dailyDate(new Date(now))}`) await this.today(now);
    const row = this.ctx.storage.sql.exec('SELECT id, name, expires_at, date, prompt FROM boards WHERE id = ?', id).toArray()[0];
    if (!row) return null;
    return { id: row.id, name: row.name, ...(row.expires_at == null ? {} : { expiresAt: new Date(row.expires_at).toISOString() }), ...(row.date == null ? {} : { date: row.date, prompt: row.prompt }) };
  }

  async list(now = Date.now()) {
    await this.today(now);
    const rows = this.ctx.storage.sql.exec('SELECT id, name, expires_at FROM boards WHERE created_at IS NOT NULL').toArray();
    const boards = [...BUILTINS.map(([id, name]) => ({ id, name })), ...rows.map(row => ({ id: row.id, name: row.name, expiresAt: new Date(row.expires_at).toISOString() })), { id: 'daily', name: 'Daily Mosaic' }];
    boards.sort((a, b) => a.name.localeCompare(b.name));
    return { boards };
  }

  async create(name, now = Date.now()) {
    if (!validBoardName(name)) throw new RangeError('Board name must contain 1–40 printable characters');
    await this.prune(now);
    const id = [...crypto.getRandomValues(new Uint8Array(12))].map(value => value.toString(16).padStart(2, '0')).join('');
    const expiresAt = customExpiry(now);
    this.ctx.storage.sql.exec('INSERT INTO boards (id, name, created_at, expires_at) VALUES (?, ?, ?, ?)', id, name.trim(), now, expiresAt);
    await this.prune(now);
    return { id, name: name.trim(), expiresAt: new Date(expiresAt).toISOString() };
  }

  async archive(now = Date.now()) {
    await this.today(now);
    const today = dailyDate(new Date(now));
    const rows = this.ctx.storage.sql.exec('SELECT id, date, prompt FROM boards WHERE date IS NOT NULL AND date < ? ORDER BY date DESC', today).toArray();
    return { boards: rows };
  }

  async alarm() { await this.prune(); }
}
