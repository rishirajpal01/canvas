import { describe, expect, it } from 'vitest';
import { randomKaleidoscope, recolorKaleidoscope } from './kaleidoscope.js';

describe('Kaleidoscope designs', () => {
  it('generates random symmetric masks with distinct designs', () => {
    const first = randomKaleidoscope();
    const second = randomKaleidoscope(first.cells);
    expect(first.cells).toHaveLength(40_000);
    expect(second.cells).toHaveLength(40_000);
    expect(first.cells).not.toEqual(second.cells);
    for (const [index, cell] of first.cells.entries()) {
      const x = index % 200;
      const y = Math.floor(index / 200);
      expect(cell).toBe(first.cells[y * 200 + (199 - x)]);
      expect(cell).toBe(first.cells[(199 - y) * 200 + x]);
    }
  });

  it('recolors the same shape without filling blocked cells', () => {
    const first = randomKaleidoscope();
    const next = recolorKaleidoscope(first.cells, first.scheme);
    expect(next.scheme).not.toEqual(first.scheme);
    for (let i = 0; i < first.cells.length; i++) {
      expect(next.cells[i]).toBe(first.cells[i] === -1 ? -1 : 0);
      if (first.cells[i] !== -1) expect(next.target[i]).toBeGreaterThan(0);
    }
  });
});
