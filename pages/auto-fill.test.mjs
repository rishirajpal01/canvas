import test from 'node:test';
import assert from 'node:assert/strict';
import {generateArtwork} from './auto-fill.mjs';

function seeded(seed) { let value = seed; return () => { value = (value * 1664525 + 1013904223) >>> 0; return value / 2 ** 32; }; }

test('all board themes create valid full artwork and preserve blocked cells', () => {
  for (const id of ['commons', 'garden', 'night-sky', 'tiny-town', 'daily', '0123456789abcdef01234567']) {
    const previous = new Array(80 * 60).fill(0);
    previous[0] = -1;
    const result = generateArtwork(id, 80, 60, previous, 'A city under the stars', seeded(57));
    assert.equal(result.length, previous.length);
    assert.equal(result[0], -1);
    assert.ok(result.filter(color => color > 0).length > 30, id);
    assert.ok(result.every(color => color >= -1 && color <= 16));
    assert.notDeepEqual(result, generateArtwork(id, 80, 60, previous, 'A city under the stars', seeded(58)));
  }
});

test('Night Sky resembles a dense Milky Way above a dark horizon', () => {
  const width = 200, height = 200;
  const previous = new Array(width * height).fill(0);
  previous[0] = -1;
  const sky = generateArtwork('night-sky', width, height, previous, '', seeded(217));
  assert.equal(sky[0], -1);
  assert.equal(sky.filter(color => color === 0).length, 0, 'Auto fill should cover the full sky');
  const upper = sky.slice(0, width * 70);
  const lowerSky = sky.slice(width * 105, width * 150);
  const horizon = sky.slice(width * 185);
  assert.ok(upper.filter(color => [2, 3, 12].includes(color)).length > upper.length * .6, 'upper sky should be dark blue and violet');
  assert.ok(lowerSky.filter(color => [4, 7, 10].includes(color)).length > upper.filter(color => [4, 7, 10].includes(color)).length / upper.length * lowerSky.length, 'violet and pink glow should strengthen toward the horizon');
  assert.ok(sky.slice(0, width * 160).filter(color => [1, 11].includes(color)).length > 150, 'sky should have many tiny bright stars');
  assert.ok(horizon.filter(color => color === 12).length > horizon.length * .6, 'bottom should be a dark silhouette');
  assert.notDeepEqual(sky, generateArtwork('night-sky', width, height, previous, '', seeded(218)));
});

test('Tiny Town has a winding road with a dashed center line below green hills', () => {
  const width = 200, height = 200;
  const town = generateArtwork('tiny-town', width, height, new Array(width * height).fill(0), '', seeded(81));
  const row = y => town.slice(y * width, (y + 1) * width);
  assert.ok(row(20).filter(color => [9, 11].includes(color)).length > 100, 'sky should be blue and cloud white');
  assert.ok(row(100).filter(color => [15, 16].includes(color)).length > 30, 'hills should be green');
  assert.ok(row(185).filter(color => color === 12).length > 45, 'road should widen toward viewer');
  assert.ok(row(150).filter(color => color === 11).length > 0, 'road should have white lane markings');
  assert.ok(town.filter(color => [5, 6, 7, 8].includes(color)).length > 200, 'large houses should add warm colors');
});

test('Pixel Garden varies its winding path and dense flowers', () => {
  const width = 120, height = 180, blank = new Array(width * height).fill(0);
  const first = generateArtwork('garden', width, height, blank, '', seeded(32));
  const second = generateArtwork('garden', width, height, blank, '', seeded(33));
  const pathColors = new Set([5, 11, 14]);
  for (const y of [30, 90, 150]) {
    assert.ok(first.slice(y * width, (y + 1) * width).filter(color => pathColors.has(color)).length > 4, `missing path at row ${y}`);
  }
  assert.ok(first.filter(color => [4, 7, 8, 10].includes(color)).length > 250, 'flowers should be abundant');
  assert.notDeepEqual(first, second, 'new seeds should change the garden');
});

test('Daily Mosaic generates three varied abstract styles from the references', () => {
  const width = 160, height = 160, blank = new Array(width * height).fill(0);
  const themed = (first, seed, boardId = 'daily-2026-09-27') => {
    const next = seeded(seed);
    let initial = true;
    return generateArtwork(boardId, width, height, blank, 'A place you want to visit', () => {
      if (initial) { initial = false; return first; }
      return next();
    });
  };
  const waves = themed(.05, 17), ovals = themed(.45, 17), flowers = themed(.85, 17);
  for (const art of [waves, ovals, flowers]) {
    assert.ok(art.every(color => color >= 1 && color <= 16), 'daily artwork should fill the board');
    assert.ok(new Set(art).size >= 6, 'daily artwork should have a broad palette');
  }
  assert.ok(waves.filter(color => color === 12).length < width * height * .05, 'color waves should avoid a dark backdrop');
  assert.ok(ovals.filter(color => color === 12).length > width * height * .2, 'oval confetti should have dark gaps');
  assert.ok(flowers.filter(color => color === 12).length > width * height * .2, 'floral wallpaper should have dark gaps');
  assert.notDeepEqual(waves, themed(.05, 18), 'each new fill should vary within its style');
  assert.notDeepEqual(ovals, flowers, 'the styles should look different');
  assert.deepEqual(themed(.45, 17), themed(.45, 17, 'daily'), 'the dated page should use Daily artwork');
});
