import test from 'node:test';
import assert from 'node:assert/strict';
import {createPixelReveal} from './pixel-reveal.mjs';

test('reveals one queued pixel per scheduled turn', async () => {
  const scheduled = [];
  const drawn = [];
  const reveal = createPixelReveal({draw: id => drawn.push(id), schedule: callback => { scheduled.push(callback); return callback; }, cancel: () => {}});
  let settled = false;
  const done = reveal.enqueue([2, 3, 4]).then(() => { settled = true; });
  assert.deepEqual(drawn, []);
  scheduled.shift()();
  await Promise.resolve();
  assert.deepEqual(drawn, [2]);
  assert.equal(settled, false);
  scheduled.shift()();
  assert.deepEqual(drawn, [2, 3]);
  scheduled.shift()();
  await done;
  assert.deepEqual(drawn, [2, 3, 4]);
});

test('hidden tabs flush pending pixels so live filling can continue', async () => {
  const scheduled = [];
  const drawn = [];
  let hidden = false;
  const reveal = createPixelReveal({draw: id => drawn.push(id), hidden: () => hidden, schedule: callback => { scheduled.push(callback); return callback; }, cancel: () => {}});
  const done = reveal.enqueue([1, 2, 3]);
  hidden = true;
  reveal.flush();
  await done;
  assert.deepEqual(drawn, [1, 2, 3]);
});
