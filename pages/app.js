import {buildToolEvents, chunkEvents, toolOverlaysArtwork} from '/board-tools.mjs';
import {freshRandom, generateArtwork} from '/auto-fill.mjs';
import {connectLive} from '/live-connection.mjs';
import {runLiveFill} from '/live-fill.mjs';
import {createPixelReveal} from '/pixel-reveal.mjs';

(() => {
  let width = 200, height = 200, total = width * height;
  const palette = ['#f1f5f9', '#005DA0', '#3A225D', '#004C93', '#B5076B', '#FF822A', '#FDB913', '#EB008B', '#DC0000', '#00AEEF', '#A288E3', '#F8F5EC', '#141B24', '#596775', '#B0ACA2', '#217B54', '#8BB95D'];
  const names = ['Blank', 'Cobalt', 'Plum', 'Deep blue', 'Berry', 'Tangerine', 'Gold', 'Pink', 'Red', 'Sky', 'Lilac', 'Cream', 'Ink', 'Slate', 'Stone', 'Forest', 'Leaf'];
  const rgb = palette.map(hex => [1, 3, 5].map(i => parseInt(hex.slice(i, i + 2), 16)));
  const routeBoard = () => location.pathname.startsWith('/board/') ? decodeURIComponent(location.pathname.slice(7)) : 'commons';
  let boardId = routeBoard();
  let dailyPage = boardId === 'daily' || /^daily-\d{4}-\d{2}-\d{2}$/.test(boardId);
  let photoBoard = /^[0-9a-f]{24}$/.test(boardId);
  const specialBoards = {
    garden: {name: 'Pixel Garden', description: 'Plant little blooms together', icon: '✿', tools: [['flower', 'Flower'], ['leaf', 'Leaf'], ['butterfly', 'Butterfly']]},
    'night-sky': {name: 'Night Sky', description: 'Build a shared constellation', icon: '✦', tools: [['star', 'Add star'], ['connect', 'Connect stars']]},
    'tiny-town': {name: 'Tiny Town', description: 'Grow a little pixel neighborhood', icon: '⌂', tools: [['house', 'House'], ['shop', 'Shop'], ['tree', 'Tree'], ['road', 'Road']]},
  };
  const api = path => path + '?board=' + encodeURIComponent(boardId);
  const $ = selector => document.querySelector(selector);
  const canvas = $('#map');
  const keyboardCursor = $('#keyboard-cursor');
  const nightMarker = $('#night-marker');
  const context = canvas.getContext('2d', {willReadFrequently: true});
  const status = $('#status');
  const colorSelect = $('#color');
  const swatches = $('#swatches');
  const fillButton = $('#auto-fill');
  const liveFillButton = $('#auto-fill-live');
  const kaleidoEmptyButton = $('#kaleido-empty');
  let cells = new Array(total).fill(0);
  let displayedColors = cells.slice();
  const reveal = createPixelReveal({draw: drawPixel, hidden: () => document.hidden});
  let target = null;
  let photoColors = null;
  let photoWidth = 200, photoHeight = 200;
  const fillJobs = new Map();
  let stream;
  let generation = 0;
  let cameraStream;
  let previewURL;
  let keyboardX = 0, keyboardY = 0;
  let activeTool = 'paint';
  let selectedStar = null;
  let placing = false;
  let readOnly = false;
  let currentDaily = null;
  let directory = null;
  let loadToken = 0;
  let actionBusy = false;
  let loading = true;

  function say(message) { status.textContent = message; status.dataset.tone = 'normal'; }
  function sayError(message) { status.textContent = message; status.dataset.tone = 'error'; }
  async function request(path, options) {
    const response = await fetch(path, options);
    if (!response.ok) {
      const error = new Error((await response.text()).trim() || 'Request failed');
      error.status = response.status;
      throw error;
    }
    return response;
  }
  function jsonOptions(method, body) {
    return {method, headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)};
  }
  function setDimensions(nextWidth, nextHeight) {
    width = nextWidth;
    height = nextHeight;
    total = width * height;
    canvas.width = width;
    canvas.height = height;
    keyboardX = Math.min(keyboardX, width - 1);
    keyboardY = Math.min(keyboardY, height - 1);
    canvas.style.setProperty('--canvas-max-width', Math.round(720 * width / height) + 'px');
    $('#canvas-size').textContent = width + ' × ' + height + ' PX';
    $('#sidebar-size').textContent = width + ' × ' + height + ' pixels';
    updateKeyboardCursor();
  }
  function updateKeyboardCursor() {
    updateNightMarker();
    if (keyboardCursor.hidden) return;
    const box = canvas.getBoundingClientRect();
    const frame = canvas.parentElement.getBoundingClientRect();
    keyboardCursor.style.left = (box.left - frame.left + keyboardX * box.width / width) + 'px';
    keyboardCursor.style.top = (box.top - frame.top + keyboardY * box.height / height) + 'px';
    keyboardCursor.style.width = (box.width / width) + 'px';
    keyboardCursor.style.height = (box.height / height) + 'px';
  }
  function updateNightMarker() {
    nightMarker.hidden = !selectedStar;
    if (!selectedStar) return;
    const box = canvas.getBoundingClientRect();
    const frame = canvas.parentElement.getBoundingClientRect();
    nightMarker.style.left = (box.left - frame.left + (selectedStar.x + .5) * box.width / width) + 'px';
    nightMarker.style.top = (box.top - frame.top + (selectedStar.y + .5) * box.height / height) + 'px';
  }
  function toolInstruction() {
    if (activeTool === 'paint') return 'Use arrow keys to choose a pixel, then Enter or Space to paint it.';
    if (boardId === 'night-sky' && activeTool === 'connect') return 'Choose two points to connect. Press Escape to cancel the first point.';
    if (boardId === 'night-sky') return 'Click the canvas, or use the keyboard cursor and Enter or Space, to add a star over the sky.';
    return 'Click the canvas, or use the keyboard cursor and Enter or Space, to place this tool over the artwork.';
  }
  canvas.addEventListener('focus', () => { keyboardCursor.hidden = false; updateKeyboardCursor(); say(readOnly ? 'Use arrow keys to inspect this archived mosaic.' : toolInstruction()); });
  canvas.addEventListener('blur', () => { keyboardCursor.hidden = true; });
  window.addEventListener('resize', updateKeyboardCursor);
  document.addEventListener('visibilitychange', () => { if (document.hidden) reveal.flush(); });
  function setFillButton() {
    const filling = fillJobs.has(boardId);
    fillButton.disabled = readOnly || loading || actionBusy;
    liveFillButton.disabled = readOnly || loading || actionBusy;
    $('#clear-board').disabled = readOnly || loading || actionBusy;
    fillButton.textContent = 'Auto Fill';
    liveFillButton.textContent = filling ? 'Stop Auto Fill Live' : 'Auto Fill Live';
    kaleidoEmptyButton.hidden = boardId !== 'kaleidoscope';
    kaleidoEmptyButton.disabled = readOnly || loading || actionBusy;
  }
  function updateProgress() {
    let paintable = 0, colored = 0;
    for (const cell of cells) { if (cell >= 0) paintable++; if (cell > 0) colored++; }
    $('#pixel-count').textContent = colored.toLocaleString() + (colored === 1 ? ' pixel placed' : ' pixels placed');
    $('#progress-bar').style.width = (paintable ? colored / paintable * 100 : 0) + '%';
    setFillButton();
  }
  function drawPixel(id) {
    context.fillStyle = cells[id] === -1 ? outsideColor : palette[cells[id]] || palette[0];
    context.fillRect(id % width, Math.floor(id / width), 1, 1);
    displayedColors[id] = cells[id];
  }
  function drawMap() {
    reveal.clear();
    const pixels = context.createImageData(width, height);
    const outside = outsideColor.startsWith('#') && outsideColor.length === 7
      ? [1, 3, 5].map(i => parseInt(outsideColor.slice(i, i + 2), 16)) : rgb[0];
    for (let id = 0; id < total; id++) {
      const color = cells[id] === -1 ? outside : rgb[cells[id]] || rgb[0];
      const offset = id * 4;
      pixels.data[offset] = color[0]; pixels.data[offset + 1] = color[1]; pixels.data[offset + 2] = color[2]; pixels.data[offset + 3] = 255;
    }
    context.putImageData(pixels, 0, 0);
    displayedColors = cells.slice();
    updateProgress();
  }
  function revealLiveEvents(events) {
    const pixels = [];
    for (const change of events) {
      cells[change.pixelId] = change.color;
      if (displayedColors[change.pixelId] !== change.color) pixels.push(change.pixelId);
    }
    updateProgress();
    return reveal.enqueue(pixels);
  }
  let outsideColor = getComputedStyle(document.documentElement).getPropertyValue('--canvas-outside').trim();
  document.addEventListener('themechange', () => {
    outsideColor = getComputedStyle(document.documentElement).getPropertyValue('--canvas-outside').trim();
    drawMap();
  });

  for (let color = 1; color < palette.length; color++) {
    const option = new Option(names[color], color);
    colorSelect.add(option);
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'swatch';
    button.style.setProperty('--swatch', palette[color]);
    button.setAttribute('aria-label', names[color]);
    button.title = names[color];
    button.addEventListener('click', () => { colorSelect.value = color; syncColor(); });
    swatches.append(button);
  }
  function syncColor() {
    const chosen = Number(colorSelect.value);
    $('#selected-color').textContent = names[chosen] + ' selected';
    [...swatches.children].forEach((button, i) => button.setAttribute('aria-pressed', i + 1 === chosen));
  }
  colorSelect.addEventListener('change', syncColor);
  syncColor();
  function configurePhoto() {
    $('#photo-section').hidden = !photoBoard;
    $('#share-kicker').textContent = photoBoard ? '03 / SHARE & SAVE' : '02 / SHARE & SAVE';
  }
  configurePhoto();

  function setupSpecialTools() {
    const config = specialBoards[boardId];
    const choices = $('#tool-choices');
    choices.replaceChildren();
    $('#special-tool-panel').hidden = !config;
    activeTool = config ? config.tools[0][0] : 'paint';
    selectedStar = null;
    updateNightMarker();
    $('#canvas-keyboard-help').textContent = toolInstruction();
    if (!config) return;
    $('#special-tool-panel').hidden = false;
    $('#special-heading').textContent = config.name + ' tools';
    const options = [...config.tools, ['paint', 'Paint one pixel']];
    for (const [tool, label] of options) {
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'tool-choice';
      button.textContent = label;
      button.setAttribute('aria-pressed', tool === activeTool ? 'true' : 'false');
      button.addEventListener('click', () => {
        activeTool = tool;
        selectedStar = null;
        updateNightMarker();
        [...choices.children].forEach(choice => choice.setAttribute('aria-pressed', choice === button ? 'true' : 'false'));
        $('#special-instruction').textContent = toolInstruction();
        $('#canvas-keyboard-help').textContent = toolInstruction();
        say(label + ' selected. ' + toolInstruction());
      });
      choices.append(button);
    }
    $('#special-instruction').textContent = toolInstruction();
    $('#canvas-keyboard-help').textContent = toolInstruction();
  }
  setupSpecialTools();

  async function loadBoard(requested = routeBoard()) {
    const token = ++loadToken;
    if (stream) { stream.close(); stream = null; }
    reveal.clear();
    placing = false;
    loading = true;
    boardId = requested;
    dailyPage = requested === 'daily' || /^daily-\d{4}-\d{2}-\d{2}$/.test(requested);
    photoBoard = /^[0-9a-f]{24}$/.test(requested);
    readOnly = false;
    actionBusy = false;
    target = null;
    photoColors = null;
    $('#photo-preview').hidden = true;
    $('#recreate').disabled = true;
    $('#daily-panel').hidden = !dailyPage;
    $('#paint-section').hidden = false;
    $('#archive-lock').hidden = true;
    $('#canvas-presence').textContent = 'Every pixel appears live for everyone on this board';
    $('#share-description').textContent = 'Invite anyone with the link to add their own pixels.';
    colorSelect.disabled = false;
    [...swatches.children].forEach(button => { button.disabled = false; });
    canvas.setAttribute('aria-label', 'Shared pixel canvas');
    configurePhoto();
    setupSpecialTools();
    setFillButton();
    say('Opening board…');
    try {
      if (dailyPage) {
        currentDaily = await (await request('/api/daily/today')).json();
        if (token !== loadToken) return;
        if (requested === 'daily') boardId = currentDaily.id;
      }
      const id = boardId;
      const [mapResponse, boardsResponse, targetResponse] = await Promise.all([
        request('/api/map?board=' + encodeURIComponent(id)), directory ? Promise.resolve(null) : request('/api/boards'), request('/api/target?board=' + encodeURIComponent(id))
      ]);
      const [map, listing, guide] = await Promise.all([mapResponse.json(), boardsResponse ? boardsResponse.json() : Promise.resolve(null), targetResponse.json()]);
      if (token !== loadToken) return;
      if (listing) directory = listing.boards;
      setDimensions(map.width, map.height);
      cells = map.cells;
      generation = map.generation;
      target = guide.colors;
      const board = directory.find(item => item.id === (dailyPage ? 'daily' : boardId));
      $('#canvas-title').textContent = dailyPage ? 'Daily Mosaic' : board ? board.name : 'Shared board';
      document.title = (dailyPage ? 'Daily Mosaic' : board ? board.name : 'Canvas') + ' — Canvas';
      renderBoards(directory);
      const expiry = $('#board-expiry');
      expiry.hidden = !board?.expiresAt;
      if (board?.expiresAt) expiry.textContent = 'This board expires ' + new Intl.DateTimeFormat('en-IN', {dateStyle: 'medium', timeStyle: 'short', timeZone: 'Asia/Kolkata'}).format(new Date(board.expiresAt)) + ' India time.';
      if (dailyPage) await renderDaily(map, token);
      if (token !== loadToken) return;
      loading = false;
      drawMap();
      say(readOnly ? 'Board ready. You can share or download it.' : 'Board ready. Choose a color and place your first pixel.');
      if (!readOnly) connectStream(token);
    } catch (error) { if (token === loadToken) sayError('Could not load board: ' + error.message); }
  }
  async function renderDaily(map, token) {
    if (token !== loadToken) return;
    readOnly = boardId !== currentDaily.id;
    $('#daily-panel').hidden = false;
    $('#daily-date').textContent = map.date + ' · INDIA TIME';
    $('#archive-lock').hidden = !readOnly;
    if (readOnly) {
      $('#canvas-presence').textContent = 'Daily Mosaic archive';
      $('#share-description').textContent = 'Share this finished mosaic or save a copy.';
    }
    colorSelect.disabled = readOnly;
    [...swatches.children].forEach(button => { button.disabled = readOnly; });
    canvas.setAttribute('aria-label', readOnly ? 'Archived read-only pixel canvas' : 'Daily Mosaic pixel canvas');
    if (readOnly) $('#canvas-keyboard-help').textContent = 'Use arrow keys to inspect the archived canvas. Painting is closed.';
    const archive = $('#daily-archive');
    archive.replaceChildren();
    try {
      const response = await request('/api/daily/archive');
      const {boards} = await response.json();
      if (token !== loadToken) return;
      for (const entry of boards) {
        const link = document.createElement('a');
        link.className = 'daily-archive-link';
        link.href = '/board/' + encodeURIComponent(entry.id);
        link.textContent = entry.date;
        link.addEventListener('click', event => {
          if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
          event.preventDefault(); navigateBoard(entry.id);
        });
        if (entry.id === boardId) link.setAttribute('aria-current', 'page');
        archive.append(link);
      }
      if (!boards.length) archive.textContent = 'The first archive will appear tomorrow.';
    } catch { if (token === loadToken) archive.textContent = 'Archive unavailable right now.'; }
  }
  function renderBoards(boards) {
    const list = $('#board-list');
    list.replaceChildren();
    const builtins = ['commons', 'kaleidoscope', 'garden', 'night-sky', 'tiny-town', 'daily'];
    boards.sort((a, b) => {
      const rank = id => builtins.includes(id) ? builtins.indexOf(id) : builtins.length;
      return rank(a.id) - rank(b.id) || a.name.localeCompare(b.name);
    });
    for (const board of boards) {
      const link = document.createElement('a');
      const active = board.id === boardId || (board.id === 'daily' && dailyPage);
      link.href = board.id === 'commons' ? '/map' : '/board/' + encodeURIComponent(board.id);
      link.addEventListener('click', event => {
        if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
        event.preventDefault(); navigateBoard(board.id);
      });
      link.className = 'board-link' + (active ? ' active' : '');
      link.setAttribute('aria-current', active ? 'page' : 'false');
      const icon = document.createElement('span');
      icon.className = 'board-icon';
      icon.textContent = board.id === 'commons' ? '✳' : board.id === 'kaleidoscope' ? '◈' : specialBoards[board.id]?.icon || (board.id === 'daily' ? '☼' : '◧');
      const copy = document.createElement('span');
      copy.className = 'board-link-copy';
      const name = document.createElement('span');
      name.textContent = board.name;
      copy.append(name);
      const description = specialBoards[board.id]?.description || (board.id === 'daily' ? 'A new canvas every India day' : '');
      if (description) {
        const small = document.createElement('small');
        small.textContent = description;
        copy.append(small);
      }
      link.append(icon, copy);
      list.append(link);
    }
  }
  function connectStream(token) {
    let connectionWarning = null;
    stream = connectLive(boardId, document.documentElement.dataset.liveTransport, update => {
      if (token !== loadToken) return;
      if (update.cells) {
        if (update.generation === generation && update.width === width && update.height === height && update.cells.every((cell, i) => cell === cells[i])) return;
        if (update.generation !== generation) {
          generation = update.generation;
          if (fillJobs.has(boardId)) {
            stopFill(boardId);
            sayError('Board changed while filling. Review it before continuing.');
          }
        }
        if (update.width && update.height && (update.width !== width || update.height !== height)) setDimensions(update.width, update.height);
        cells = update.cells;
        if ('target' in update) target = update.target;
        drawMap();
      } else if (update.events) {
        if (update.live) revealLiveEvents(update.events);
        else {
          for (const change of update.events) { cells[change.pixelId] = change.color; drawPixel(change.pixelId); }
          updateProgress();
        }
      } else if (Number.isInteger(update.pixelId)) {
        cells[update.pixelId] = update.color;
        drawPixel(update.pixelId);
        updateProgress();
      }
    }, message => {
      if (token !== loadToken) return;
      if (message) { connectionWarning = message; sayError(message); }
      else {
        if (connectionWarning && status.textContent === connectionWarning) say('Connected. Live updates resumed.');
        connectionWarning = null;
      }
    });
  }

  function navigateBoard(id, push = true) {
    const url = id === 'commons' ? '/map' : '/board/' + encodeURIComponent(id);
    if (push) history.pushState({}, '', url);
    loadBoard(id);
  }
  $('.brand').addEventListener('click', event => {
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault(); navigateBoard('commons');
  });
  window.addEventListener('popstate', () => loadBoard(routeBoard()));
  setInterval(async () => {
    if (!dailyPage || location.pathname !== '/board/daily') return;
    try {
      const next = await (await request('/api/daily/today')).json();
      if (next.id !== boardId) loadBoard('daily');
    } catch { sayError('Could not check for the next daily board. Retrying shortly.'); }
  }, 60_000);
  setInterval(async () => {
    if (!directory) return;
    try {
      const response = await request('/api/boards');
      directory = (await response.json()).boards;
      if (photoBoard && !directory.some(board => board.id === boardId)) {
        navigateBoard('commons');
        say('That custom board reached its seven-day expiry.');
      } else renderBoards(directory);
    } catch { /* Keep the current sidebar until the next refresh. */ }
  }, 60_000);

  async function paintPixel(x, y) {
    if (loading) return;
    if (readOnly) { sayError('This mosaic is archived and read only.'); return; }
    const pixelId = y * width + x;
    if (cells[pixelId] === -1) { sayError('This pixel is outside the kaleidoscope pattern.'); return; }
    const paintGeneration = generation, token = loadToken, id = boardId;
    try {
      await request('/api/events?board=' + encodeURIComponent(id), jsonOptions('POST', {pixelId, color: Number(colorSelect.value), generation: paintGeneration}));
      if (generation !== paintGeneration || token !== loadToken) return;
      cells[pixelId] = Number(colorSelect.value);
      drawPixel(pixelId);
      updateProgress();
      say('Placed ' + names[Number(colorSelect.value)].toLowerCase() + ' at (' + x + ', ' + y + ').');
    } catch (error) { sayError('Could not place pixel: ' + error.message); }
  }
  async function placeSpecialTool(x, y) {
    if (loading) return;
    if (placing) return;
    if (boardId === 'night-sky' && activeTool === 'connect' && !selectedStar) {
      selectedStar = {x, y};
      updateNightMarker();
      say('First star selected at (' + x + ', ' + y + '). Choose the second point or press Escape.');
      return;
    }
    const origin = selectedStar;
    selectedStar = null;
    updateNightMarker();
    const events = buildToolEvents(boardId, activeTool, x, y, Number(colorSelect.value), width, height, origin);
    if (!events.length) { sayError('Choose a point on the board to use this tool.'); return; }
    const placementGeneration = generation, token = loadToken, id = boardId;
    const overlay = toolOverlaysArtwork(boardId);
    placing = true;
    let placed = 0;
    try {
      for (const batch of chunkEvents(events)) {
        if (token !== loadToken) return;
        const response = await request('/api/events/batch?board=' + encodeURIComponent(id), jsonOptions('POST', {events: batch, generation: placementGeneration, onlyBlank: !overlay}));
        const result = await response.json();
        if (generation !== placementGeneration || token !== loadToken) throw new Error('Board changed. Try again.');
        for (const change of result.events) {
          if (overlay || cells[change.pixelId] === 0) {
            cells[change.pixelId] = change.color;
            drawPixel(change.pixelId);
          }
        }
        placed += result.events.length;
        updateProgress();
      }
      if (placed) say('Placed ' + placed + (placed === 1 ? ' pixel.' : ' pixels.') + (boardId === 'night-sky' ? ' The constellation is visible over the sky.' : overlay ? ' Existing artwork was replaced.' : ' Existing artwork was preserved.'));
      else sayError('That area is already painted. Try another blank spot.');
    } catch (error) { sayError('Could not finish this tool: ' + error.message); }
    finally { placing = false; }
  }
  function activateCanvas(x, y) {
    if (readOnly) { sayError('This mosaic is archived and read only.'); return; }
    if (activeTool === 'paint') paintPixel(x, y);
    else placeSpecialTool(x, y);
  }
  canvas.addEventListener('click', event => {
    const box = canvas.getBoundingClientRect();
    const x = Math.max(0, Math.min(width - 1, Math.floor((event.clientX - box.left) * width / box.width)));
    const y = Math.max(0, Math.min(height - 1, Math.floor((event.clientY - box.top) * height / box.height)));
    keyboardX = x;
    keyboardY = y;
    updateKeyboardCursor();
    activateCanvas(x, y);
  });
  canvas.addEventListener('keydown', event => {
    const moves = {ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1]};
    if (moves[event.key]) {
      event.preventDefault();
      keyboardX = Math.max(0, Math.min(width - 1, keyboardX + moves[event.key][0]));
      keyboardY = Math.max(0, Math.min(height - 1, keyboardY + moves[event.key][1]));
      updateKeyboardCursor();
      say(readOnly ? 'Pixel (' + keyboardX + ', ' + keyboardY + ') on a read-only mosaic.' : 'Pixel (' + keyboardX + ', ' + keyboardY + '). Press Enter or Space to use ' + (activeTool === 'paint' ? 'paint' : activeTool) + '.');
    } else if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      activateCanvas(keyboardX, keyboardY);
    } else if (event.key === 'Escape' && selectedStar) {
      event.preventDefault();
      selectedStar = null;
      updateNightMarker();
      say('Constellation start canceled. Choose another first point.');
    }
  });

  const createDialog = $('#create-dialog');
  const openCreate = () => { $('#create-status').textContent = ''; createDialog.showModal(); $('#board-name').focus(); };
  $('#create-top').addEventListener('click', openCreate);
  $('#create-side').addEventListener('click', openCreate);
  $('#close-create').addEventListener('click', () => createDialog.close());
  $('#create-form').addEventListener('submit', async event => {
    event.preventDefault();
    const name = $('#board-name').value.trim();
    if (!name) return;
    try {
      const response = await request('/api/boards', jsonOptions('POST', {name}));
      const board = await response.json();
      directory = null;
      createDialog.close();
      navigateBoard(board.id);
    } catch (error) { $('#create-status').textContent = 'Could not create board: ' + error.message; }
  });

  $('#copy-link').addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(location.href);
      say(readOnly ? 'Archive link copied.' : 'Board link copied. Share it with anyone you want to paint with.');
    } catch { say('Copy this board link: ' + location.href); }
  });
  $('#download').addEventListener('click', () => {
    const source = document.createElement('canvas');
    source.width = width;
    source.height = height;
    const sourceContext = source.getContext('2d');
    const pixels = sourceContext.createImageData(width, height);
    for (let id = 0; id < total; id++) {
      if (cells[id] === -1) continue;
      const color = rgb[cells[id]] || rgb[0];
      pixels.data.set([...color, 255], id * 4);
    }
    sourceContext.putImageData(pixels, 0, 0);
    const exportCanvas = document.createElement('canvas');
    const scale = 1000 / Math.max(width, height);
    exportCanvas.width = Math.max(1, Math.round(width * scale));
    exportCanvas.height = Math.max(1, Math.round(height * scale));
    const exportContext = exportCanvas.getContext('2d');
    exportContext.imageSmoothingEnabled = false;
    exportContext.drawImage(source, 0, 0, exportCanvas.width, exportCanvas.height);
    exportCanvas.toBlob(blob => {
      if (!blob) { sayError('Could not prepare the download.'); return; }
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = (boardId === 'commons' ? 'the-commons' : boardId) + '.png';
      link.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
      say('PNG downloaded at ' + exportCanvas.width + ' × ' + exportCanvas.height + ' pixels.');
    }, 'image/png');
  });

  function closestColor(r, g, b) {
    let best = 1, score = Infinity;
    for (let i = 1; i < rgb.length; i++) {
      const color = rgb[i];
      const distance = 0.3 * (r - color[0]) ** 2 + 0.59 * (g - color[1]) ** 2 + 0.11 * (b - color[2]) ** 2;
      if (distance < score) { score = distance; best = i; }
    }
    return best;
  }
  function preparePhoto(image, label) {
    const scale = Math.min(1, 512 / Math.max(image.width, image.height), Math.sqrt(40000 / (image.width * image.height)));
    photoWidth = Math.max(1, Math.floor(image.width * scale));
    photoHeight = Math.max(1, Math.floor(image.height * scale));
    const sample = document.createElement('canvas');
    sample.width = photoWidth;
    sample.height = photoHeight;
    const ctx = sample.getContext('2d', {willReadFrequently: true});
    ctx.drawImage(image, 0, 0, photoWidth, photoHeight);
    const data = ctx.getImageData(0, 0, photoWidth, photoHeight).data;
    photoColors = new Array(photoWidth * photoHeight);
    for (let i = 0; i < photoColors.length; i++) {
      const a = data[i * 4 + 3] / 255;
      photoColors[i] = closestColor(data[i * 4] * a + 255 * (1 - a), data[i * 4 + 1] * a + 255 * (1 - a), data[i * 4 + 2] * a + 255 * (1 - a));
    }
    $('#preview-image').src = sample.toDataURL('image/png');
    $('#preview-label').textContent = label + ' · ' + photoWidth + ' × ' + photoHeight + ' pixels';
    $('#photo-preview').hidden = false;
    $('#recreate').disabled = false;
    say('Photo ready. Recreate it to replace this board and watch the pixels appear.');
  }
  $('#upload').addEventListener('change', async event => {
    const file = event.target.files[0];
    if (!file) return;
    if (previewURL) URL.revokeObjectURL(previewURL);
    previewURL = URL.createObjectURL(file);
    const image = new Image();
    image.src = previewURL;
    try { await image.decode(); preparePhoto(image, file.name); }
    catch (error) { sayError('Could not read photo: ' + error.message); }
  });

  const cameraDialog = $('#camera-dialog');
  async function stopCamera() {
    if (cameraStream) { cameraStream.getTracks().forEach(track => track.stop()); cameraStream = null; }
    $('#camera-video').srcObject = null;
  }
  $('#open-camera').addEventListener('click', async () => {
    cameraDialog.showModal();
    $('#camera-status').textContent = 'Opening camera…';
    try {
      cameraStream = await navigator.mediaDevices.getUserMedia({video: {facingMode: 'environment'}, audio: false});
      $('#camera-video').srcObject = cameraStream;
      $('#camera-status').textContent = 'Frame your photo, then capture it.';
    } catch (error) { $('#camera-status').textContent = 'Camera unavailable: ' + error.message + '. You can upload a photo instead.'; }
  });
  $('#capture').addEventListener('click', () => {
    const video = $('#camera-video');
    if (!video.videoWidth) { $('#camera-status').textContent = 'Camera is still opening.'; return; }
    const frame = document.createElement('canvas');
    frame.width = video.videoWidth;
    frame.height = video.videoHeight;
    frame.getContext('2d').drawImage(video, 0, 0);
    preparePhoto(frame, 'Camera photo');
    cameraDialog.close();
  });
  $('#close-camera').addEventListener('click', () => cameraDialog.close());
  cameraDialog.addEventListener('close', stopCamera);

  function stopFill(id) {
    const job = fillJobs.get(id);
    if (!job) return;
    job.active = false;
    fillJobs.delete(id);
    if (id === boardId) setFillButton();
  }
  async function startFillJob(id, fillGeneration, initialCells, targetColors, fillWidth, fillHeight) {
    stopFill(id);
    const job = {active: true, generation: fillGeneration};
    fillJobs.set(id, job);
    if (id === boardId) setFillButton();
    try {
      const result = await runLiveFill({
        boardId: id, generation: job.generation, cells: initialCells, target: targetColors, width: fillWidth, height: fillHeight,
        isActive: () => job.active,
        send: (board, expected, events) => request('/api/events/batch?board=' + encodeURIComponent(board), jsonOptions('POST', {events, generation: expected, live: true})),
        onBatch: (events, placed, totalToPlace) => {
          if (boardId !== id || loading || generation !== job.generation) return;
          const revealed = revealLiveEvents(events);
          say('Building live · ' + placed.toLocaleString() + ' / ' + totalToPlace.toLocaleString() + ' pixels');
          return revealed;
        },
      });
      if (!result.cancelled && boardId === id) say('Fill complete. Every pixel is ready to share or download.');
    } catch (error) { if (job.active && boardId === id) sayError('Fill paused: ' + error.message); }
    finally {
      if (fillJobs.get(id) === job) {
        fillJobs.delete(id);
        if (boardId === id) setFillButton();
      }
    }
  }
  function fillFromTarget() {
    if (!target || target.length !== total) return;
    startFillJob(boardId, generation, cells, target, width, height);
  }
  async function refreshArtwork(id, token) {
    const [mapResponse, targetResponse] = await Promise.all([
      request('/api/map?board=' + encodeURIComponent(id)), request('/api/target?board=' + encodeURIComponent(id))
    ]);
    const [map, guide] = await Promise.all([mapResponse.json(), targetResponse.json()]);
    if (token !== loadToken || id !== boardId) return;
    if (map.width !== width || map.height !== height) setDimensions(map.width, map.height);
    cells = map.cells;
    target = guide.colors;
    generation = map.generation;
    drawMap();
  }
  async function boardAction(kind) {
    if (readOnly || loading) return;
    const id = boardId, token = loadToken, expected = generation;
    if (actionBusy) return;
    const hasPaint = cells.some(cell => cell > 0);
    const question = kind === 'clear' ? 'Clear all paint on this board?' : kind === 'empty' ? 'Build a new empty Kaleidoscope?' : kind === 'live' ? 'Replace all paint and fill this board live, one pixel at a time?' : 'Replace all paint with a new Auto Fill design?';
    if ((kind === 'clear' || hasPaint) && !confirm(question)) return;
    stopFill(id);
    actionBusy = true;
    setFillButton();
    try {
      let liveState = null;
      if (kind === 'clear') {
        await request('/api/clear?board=' + encodeURIComponent(id), jsonOptions('POST', {generation: expected}));
      } else if (id === 'kaleidoscope') {
        if (kind === 'fill') {
          await request('/api/kaleidoscope/autofill?board=kaleidoscope', jsonOptions('POST', {generation: expected}));
        } else {
          const reroll = await request('/api/kaleidoscope/reroll?board=kaleidoscope', jsonOptions('POST', {generation: expected}));
          if (kind === 'live') {
            const {generation: emptyGeneration} = await reroll.json();
            const recolor = await request('/api/kaleidoscope/recolor?board=kaleidoscope', jsonOptions('POST', {generation: emptyGeneration}));
            const {generation: fillGeneration} = await recolor.json();
            const [map, guide] = await Promise.all([
              request('/api/map?board=kaleidoscope').then(response => response.json()),
              request('/api/target?board=kaleidoscope').then(response => response.json()),
            ]);
            if (map.generation !== fillGeneration) throw new Error('The board changed. Try again.');
            liveState = {generation: fillGeneration, cells: map.cells, target: guide.colors, width: map.width, height: map.height};
          }
        }
      } else {
        const artwork = generateArtwork(id, width, height, cells, '', freshRandom());
        const nextCells = kind === 'live' ? cells.map(cell => cell === -1 ? -1 : 0) : artwork;
        const response = await request('/api/artwork?board=' + encodeURIComponent(id), jsonOptions('PUT', {generation: expected, cells: nextCells}));
        if (kind === 'live') {
          const {generation: fillGeneration} = await response.json();
          liveState = {generation: fillGeneration, cells: nextCells, target: artwork, width, height};
        }
      }
      if (token === loadToken && id === boardId) {
        if (liveState) {
          if (liveState.width !== width || liveState.height !== height) setDimensions(liveState.width, liveState.height);
          cells = liveState.cells;
          target = liveState.target;
          generation = liveState.generation;
          drawMap();
        } else {
          await refreshArtwork(id, token);
          say(kind === 'clear' ? 'Board cleared.' : kind === 'empty' ? 'Empty Kaleidoscope ready.' : 'New artwork is ready.');
        }
      }
      if (liveState) startFillJob(id, liveState.generation, liveState.cells, liveState.target, liveState.width, liveState.height);
    } catch (error) {
      if (token === loadToken) {
        if (error.status === 409) {
          try { await refreshArtwork(id, token); sayError('The board changed. Latest artwork loaded; try again.'); }
          catch { sayError('The board changed. Reopen it before trying again.'); }
        } else sayError('Could not ' + (kind === 'clear' ? 'clear board' : kind === 'empty' ? 'build Kaleidoscope' : 'auto fill') + ': ' + error.message);
      }
    }
    finally { if (token === loadToken) { actionBusy = false; setFillButton(); } }
  }
  fillButton.addEventListener('click', () => boardAction('fill'));
  liveFillButton.addEventListener('click', () => {
    if (fillJobs.has(boardId)) { stopFill(boardId); reveal.flush(); say('Live fill stopped.'); return; }
    boardAction('live');
  });
  kaleidoEmptyButton.addEventListener('click', () => boardAction('empty'));
  $('#clear-board').addEventListener('click', () => boardAction('clear'));
  $('#recreate').addEventListener('click', async () => {
    if (!photoColors) return;
    stopFill(boardId);
    $('#recreate').disabled = true;
    say('Preparing the new canvas…');
    try {
      const photo = await request(api('/api/photo'), jsonOptions('PUT', {width: photoWidth, height: photoHeight, colors: photoColors}));
      generation = (await photo.json()).generation;
      setDimensions(photoWidth, photoHeight);
      target = photoColors.slice();
      cells = new Array(total).fill(0);
      drawMap();
      fillFromTarget();
    } catch (error) { sayError('Could not recreate photo: ' + error.message); }
    finally { $('#recreate').disabled = false; }
  });

  loadBoard();
})();
