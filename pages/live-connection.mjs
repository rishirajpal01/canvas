export function connectLive(boardId, transport, onUpdate, onStatus, dependencies = {}) {
  const NativeWebSocket = dependencies.WebSocket ?? globalThis.WebSocket;
  const NativeEventSource = dependencies.EventSource ?? globalThis.EventSource;
  const origin = dependencies.origin ?? globalThis.location?.origin;
  const later = dependencies.setTimeout ?? globalThis.setTimeout;
  const cancel = dependencies.clearTimeout ?? globalThis.clearTimeout;
  let active = true;
  let socket;
  let reconnectTimer;
  const board = encodeURIComponent(boardId);

  const receive = event => {
    if (!active) return;
    try { onUpdate(JSON.parse(event.data)); onStatus(null); }
    catch { onStatus('Received an invalid board update. Reconnecting…'); }
  };

  if (transport === 'websocket') {
    const connect = () => {
      if (!active) return;
      const url = new URL(`/api/live?board=${board}`, origin);
      url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
      socket = new NativeWebSocket(url.href);
      socket.onmessage = receive;
      socket.onerror = () => { if (active) onStatus('Connection lost. Reconnecting automatically…'); };
      socket.onclose = () => {
        if (!active) return;
        onStatus('Connection lost. Reconnecting automatically…');
        reconnectTimer = later(connect, 1000);
      };
    };
    connect();
  } else {
    socket = new NativeEventSource(`/api/stream?board=${board}`);
    socket.onmessage = receive;
    socket.onerror = () => { if (active) onStatus('Connection lost. Reconnecting automatically…'); };
  }

  return {
    close() {
      active = false;
      if (reconnectTimer !== undefined) cancel(reconnectTimer);
      socket?.close();
    },
  };
}
