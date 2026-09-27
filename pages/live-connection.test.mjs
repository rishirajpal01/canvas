import test from 'node:test';
import assert from 'node:assert/strict';
import { connectLive } from './live-connection.mjs';

test('WebSocket transport forwards snapshots and reconnects after a drop', async () => {
  const sockets = [];
  class Socket {
    constructor(url) { this.url = url; sockets.push(this); }
    close() { this.closed = true; }
  }
  const updates = [];
  const statuses = [];
  const connection = connectLive('garden', 'websocket', value => updates.push(value), status => statuses.push(status), {
    WebSocket: Socket, origin: 'https://canvas.example', setTimeout: callback => { callback(); return 1; }, clearTimeout: () => {},
  });
  assert.equal(sockets[0].url, 'wss://canvas.example/api/live?board=garden');
  sockets[0].onmessage({ data: JSON.stringify({ width: 200, cells: [0], generation: 1 }) });
  assert.equal(updates[0].generation, 1);
  sockets[0].onclose();
  assert.equal(sockets.length, 2);
  sockets[1].onmessage({ data: JSON.stringify({ width: 200, cells: [0], generation: 1 }) });
  assert.deepEqual(statuses, [null, 'Connection lost. Reconnecting automatically…', null]);
  connection.close();
  assert.equal(sockets[1].closed, true);
});

test('SSE transport keeps the Go server endpoint and closes on navigation', () => {
  let source;
  class EventSource {
    constructor(url) { this.url = url; source = this; }
    close() { this.closed = true; }
  }
  const updates = [];
  const statuses = [];
  const connection = connectLive('commons', 'sse', value => updates.push(value), status => statuses.push(status), { EventSource });
  assert.equal(source.url, '/api/stream?board=commons');
  source.onerror();
  source.onmessage({ data: JSON.stringify({ pixelId: 2, color: 3 }) });
  assert.deepEqual(updates, [{ pixelId: 2, color: 3 }]);
  assert.deepEqual(statuses, ['Connection lost. Reconnecting automatically…', null]);
  connection.close();
  assert.equal(source.closed, true);
});
