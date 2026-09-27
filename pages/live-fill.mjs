import {chunkEvents, kaleidoscopeFillBatches} from './board-tools.mjs';

export async function runLiveFill({boardId, generation, cells, target, width, height, isActive, send, onBatch}) {
  const current = cells.slice();
  const batches = boardId === 'kaleidoscope'
    ? kaleidoscopeFillBatches(current, target, width, height)
    : chunkEvents(current.flatMap((color, pixelId) => color === 0 && target[pixelId] > 0 ? [{pixelId, color: target[pixelId]}] : []));
  const total = batches.reduce((sum, batch) => sum + batch.length, 0);
  let placed = 0;
  for (const batch of batches) {
    if (!isActive()) return {placed, total, cancelled: true};
    const events = boardId === 'kaleidoscope' ? batch : batch.filter(({pixelId}) => current[pixelId] === 0);
    if (!events.length) continue;
    await send(boardId, generation, events);
    if (!isActive()) return {placed, total, cancelled: true};
    for (const change of events) current[change.pixelId] = change.color;
    placed += events.length;
    await onBatch(events, placed, total);
  }
  return {placed, total, cancelled: false};
}
