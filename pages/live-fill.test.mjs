import test from 'node:test';
import assert from 'node:assert/strict';
import {runLiveFill} from './live-fill.mjs';

test('live fill keeps sending to its starting board while another board is selected', async () => {
  const sent = [];
  const progress = [];
  let visibleBoard = 'garden';
  const cells = Array(130).fill(0);
  const target = Array.from({length: 130}, (_, index) => index % 4 + 1);
  const result = await runLiveFill({
    boardId: 'garden', generation: 7, cells, target, width: 13, height: 10,
    isActive: () => true,
    send: async (id, generation, events) => {
      sent.push({id, generation, events});
      if (sent.length === 1) visibleBoard = 'tiny-town';
    },
    onBatch: (events, placed, total) => progress.push({visibleBoard, events, placed, total}),
  });

  assert.equal(result.cancelled, false);
  assert.equal(result.placed, 130);
  assert.deepEqual(sent.map(batch => [batch.id, batch.generation]), [['garden', 7], ['garden', 7], ['garden', 7]]);
  assert.deepEqual(progress.map(batch => batch.visibleBoard), ['tiny-town', 'tiny-town', 'tiny-town']);
});

test('stopping a fill does not send another batch', async () => {
  let active = true;
  const sent = [];
  const result = await runLiveFill({
    boardId: 'garden', generation: 3, cells: Array(130).fill(0), target: Array(130).fill(1), width: 13, height: 10,
    isActive: () => active,
    send: async (id, generation, events) => { sent.push(events); active = false; },
    onBatch: () => {},
  });
  assert.equal(result.cancelled, true);
  assert.equal(sent.length, 1);
});

test('live fill waits for each batch to be revealed before sending the next', async () => {
  const order = [];
  let revealNext;
  const finished = runLiveFill({
    boardId: 'commons', generation: 2, cells: Array(65).fill(0), target: Array(65).fill(1), width: 13, height: 5,
    isActive: () => true,
    send: async () => { order.push('send'); },
    onBatch: () => new Promise(resolve => { revealNext = () => { order.push('revealed'); resolve(); }; }),
  });
  await new Promise(resolve => setImmediate(resolve));
  assert.deepEqual(order, ['send']);
  revealNext();
  await new Promise(resolve => setImmediate(resolve));
  assert.deepEqual(order, ['send', 'revealed', 'send']);
  revealNext();
  await finished;
});
