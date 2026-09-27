import { describe, expect, it } from 'vitest';
import { env } from 'cloudflare:workers';
import { dailyDate, dailyCutoff, customExpiry, validBoardName, validBoardId } from './catalogue.js';

describe('catalogue rules', () => {
  it('changes Daily Mosaic at India midnight and keeps seven dates', () => {
    const before = new Date('2026-09-26T18:29:59Z');
    const after = new Date('2026-09-26T18:30:00Z');
    expect(dailyDate(before)).toBe('2026-09-26');
    expect(dailyDate(after)).toBe('2026-09-27');
    expect(dailyCutoff(after)).toBe('2026-09-21');
  });

  it('expires custom boards exactly 168 hours after creation', () => {
    const created = Date.parse('2026-09-20T00:00:00Z');
    expect(customExpiry(created)).toBe(Date.parse('2026-09-27T00:00:00Z'));
  });

  it('accepts only supported board IDs and printable names', () => {
    expect(validBoardId('commons')).toBe(true);
    expect(validBoardId('daily-2026-09-27')).toBe(true);
    expect(validBoardId('a'.repeat(24))).toBe(true);
    expect(validBoardId('../secret')).toBe(false);
    expect(validBoardName(' Our board ')).toBe(true);
    expect(validBoardName('\u0000bad')).toBe(false);
    expect(validBoardName('x'.repeat(41))).toBe(false);
  });
});

describe('catalogue storage', () => {
  it('creates, lists, and expires a user board', async () => {
    const catalogue = env.CATALOGUE.getByName(crypto.randomUUID());
    const created = Date.parse('2026-09-20T00:00:00Z');
    const board = await catalogue.create('Our board', created);
    expect(board.id).toMatch(/^[a-f0-9]{24}$/);
    expect((await catalogue.list(created)).boards).toContainEqual(board);
    expect(await catalogue.resolve(board.id, created + 7 * 86_400_000)).toBeNull();
  });

  it('keeps six archived India days and rejects the seventh', async () => {
    const catalogue = env.CATALOGUE.getByName(crypto.randomUUID());
    const base = Date.parse('2026-09-20T18:30:00Z');
    for (let i = 0; i < 8; i++) await catalogue.today(base + i * 86_400_000);
    const archive = await catalogue.archive(base + 7 * 86_400_000);
    expect(archive.boards).toHaveLength(6);
    expect(archive.boards.map(board => board.date)).toContain('2026-09-22');
    expect(archive.boards.map(board => board.date)).not.toContain('2026-09-21');
  });
});
