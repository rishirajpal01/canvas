export function createPixelReveal({draw, hidden = () => false, schedule = callback => setTimeout(callback, 0), cancel = clearTimeout}) {
  let queue = [];
  let next = 0;
  let timer = null;
  let waiters = [];
  const pending = new Set();

  function finish() {
    queue = [];
    next = 0;
    pending.clear();
    for (const resolve of waiters) resolve();
    waiters = [];
  }
  function stopTimer() {
    if (timer !== null) { cancel(timer); timer = null; }
  }
  function flush() {
    stopTimer();
    while (next < queue.length) draw(queue[next++]);
    finish();
  }
  function clear() {
    stopTimer();
    finish();
  }
  function tick() {
    timer = null;
    if (hidden()) { flush(); return; }
    const id = queue[next++];
    pending.delete(id);
    draw(id);
    if (next < queue.length) timer = schedule(tick);
    else finish();
  }
  function enqueue(ids) {
    if (hidden()) {
      flush();
      for (const id of ids) draw(id);
      return Promise.resolve();
    }
    for (const id of ids) if (!pending.has(id)) { pending.add(id); queue.push(id); }
    if (next >= queue.length) return Promise.resolve();
    if (timer === null) timer = schedule(tick);
    return new Promise(resolve => waiters.push(resolve));
  }
  return {enqueue, flush, clear};
}
