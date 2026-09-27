import test from 'node:test';
import assert from 'node:assert/strict';
import {buildToolEvents, chunkEvents, kaleidoscopeFillBatches, toolOverlaysArtwork} from './board-tools.mjs';

const choices = {
  garden: ['flower', 'leaf', 'butterfly'],
  'night-sky': ['star', 'connect'],
  'tiny-town': ['house', 'shop', 'tree', 'road'],
};

test('each special tool produces unique, in-bounds events at every board edge', () => {
  for (const [board, tools] of Object.entries(choices)) {
    for (const tool of tools) {
      for (const [x, y] of [[0, 0], [199, 0], [0, 199], [199, 199]]) {
        const origin = tool === 'connect' ? {x: 100, y: 100} : null;
        const events = buildToolEvents(board, tool, x, y, 6, 200, 200, origin);
        assert.ok(events.length > 0, `${board}/${tool} empty at ${x},${y}`);
        assert.equal(new Set(events.map(event => event.pixelId)).size, events.length, `${board}/${tool} duplicated a pixel`);
        assert.ok(events.every(({pixelId, color}) => pixelId >= 0 && pixelId < 40000 && color >= 1 && color <= 16), `${board}/${tool} escaped board or palette`);
        if (board === 'garden') assert.ok(events.length <= 49);
        if (board === 'tiny-town') assert.ok(events.length <= 64);
      }
    }
  }
});

test('Garden and Town tools replace existing artwork like Night Sky tools', () => {
  for (const id of ['garden', 'tiny-town', 'night-sky']) assert.equal(toolOverlaysArtwork(id), true, id);
  assert.equal(toolOverlaysArtwork('kaleidoscope'), false);
  assert.equal(toolOverlaysArtwork('commons'), false);
});

test('Night Sky stamps five distinct star points with open corners', () => {
  const events = buildToolEvents('night-sky', 'star', 50, 50, 6, 200, 200);
  const cells = new Map(events.map(({pixelId, color}) => [pixelId, color]));
  const at = (dx, dy) => cells.get((50 + dy) * 200 + 50 + dx);
  for (const [dx, dy] of [[0, -5], [-5, -2], [5, -2], [-4, 5], [4, 5]]) {
    assert.ok(at(dx, dy), `missing star point at ${dx},${dy}`);
  }
  for (const [dx, dy] of [[-5, -5], [5, -5], [0, 5]]) {
    assert.equal(at(dx, dy), undefined, `star gap filled at ${dx},${dy}`);
  }
  assert.equal(at(0, 0), 11, 'the star keeps its contrasting center');
  assert.ok(events.length > 9 && events.length <= 64, 'one click creates a visible star in one batch');
});

test('a constellation line reaches both points and chunks into sixty-four event batches', () => {
  const events = buildToolEvents('night-sky', 'connect', 199, 199, 2, 200, 200, {x: 0, y: 0});
  assert.equal(events.length, 200);
  assert.equal(events[0].pixelId, 0);
  assert.equal(events.at(-1).pixelId, 39999);
  const batches = chunkEvents(events);
  assert.equal(batches.length, 4);
  assert.ok(batches.every(batch => batch.length > 0 && batch.length <= 64));
  assert.deepEqual(batches.flat(), events);
});

test('Kaleidoscope fill sends complete symmetric orbits in every batch', () => {
  const size = 12, cells = new Array(size * size).fill(0);
  const target = cells.map((_, id) => 1 + (Math.min(id % size, size - 1 - id % size, Math.floor(id / size), size - 1 - Math.floor(id / size)) % 4));
  const batches = kaleidoscopeFillBatches(cells, target, size, size);
  assert.ok(batches.length > 1);
  const painted = cells.slice();
  for (const batch of batches) {
    assert.ok(batch.length <= 64);
    for (const {pixelId, color} of batch) painted[pixelId] = color;
    for (let y = 0; y < size; y++) for (let x = 0; x < size; x++) {
      const value = painted[y * size + x];
      assert.equal(value, painted[y * size + size - 1 - x]);
      assert.equal(value, painted[(size - 1 - y) * size + x]);
      assert.equal(value, painted[x * size + y]);
    }
  }
  assert.deepEqual(painted, target);
});
