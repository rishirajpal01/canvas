const motifs = {
  garden: {
    flower: [
      '..C.C..', '.CCMCC.', 'CCMMMCC', '.MMMMM.', 'CCMMMCC', '.CCMCC.', '..C.C..',
    ],
    leaf: [
      '......M', '....MM.', '...MMM.', '..MMMM.', '.MMMM..', 'MMMM...', 'M......',
    ],
    butterfly: [
      'MM...MM', 'MMM.MMM', '.MMMMM.', '..MIM..', '.MMMMM.', 'MMM.MMM', 'MM...MM',
    ],
  },
  'night-sky': {
    star: [
      '.....M.....', '....MMM....', '...MMMMM...', 'MMMMMMMMMMM', '.MMMMMMMMM.',
      '..MMMCMMM..', '...MMMMM...', '..MMMMMMM..', '..MMM.MMM..', '.MMM...MMM.', '.MM.....MM.',
    ],
  },
  'tiny-town': {
    house: [
      '...MM...', '..MMMM..', '.MMMMMM.', 'MMMMMMMM', '.CCCCCC.', '.CMCCMC.', '.CMCCMC.', '.CCCCCC.',
    ],
    shop: [
      '.MMMMMM.', '.MMMMMM.', '.CICICI.', '.CCCCCC.', '.CCIICCC', '.CCIICCC', '.CCCCCC.', '.MMMMMM.',
    ],
    tree: [
      '...LL...', '..LLLL..', '.LLLLLL.', 'LLLLLLLL', '.LLLLLL.', '..LII...', '...I....', '...I....',
    ],
    road: [
      'I......I', 'I...C..I', 'I......I', 'I......I', 'I...C..I', 'I......I', 'I......I', 'I...C..I',
    ],
  },
};

function validPoint(x, y) { return Number.isInteger(x) && Number.isInteger(y); }

export function toolOverlaysArtwork(boardId) {
  return boardId === 'garden' || boardId === 'tiny-town' || boardId === 'night-sky';
}

export function buildToolEvents(boardId, tool, x, y, color, width, height, origin = null) {
  if (!validPoint(x, y) || !validPoint(width, height) || width <= 0 || height <= 0 || !Number.isInteger(color) || color < 1 || color > 16) return [];
  const events = new Map();
  const contrast = color === 11 ? 12 : 11;
  function add(px, py, shade = color) {
    if (px >= 0 && px < width && py >= 0 && py < height) events.set(py * width + px, shade);
  }
  if (boardId === 'night-sky' && tool === 'connect' && origin && validPoint(origin.x, origin.y)) {
    let px = origin.x, py = origin.y;
    const dx = Math.abs(x - px), sx = px < x ? 1 : -1;
    const dy = -Math.abs(y - py), sy = py < y ? 1 : -1;
    let error = dx + dy;
    while (true) {
      add(px, py);
      if (px === x && py === y) break;
      const twice = 2 * error;
      if (twice >= dy) { error += dy; px += sx; }
      if (twice <= dx) { error += dx; py += sy; }
    }
  } else {
    const rows = motifs[boardId]?.[tool];
    if (rows) {
      const left = x - Math.floor(rows[0].length / 2);
      const top = y - Math.floor(rows.length / 2);
      rows.forEach((row, dy) => {
        for (let dx = 0; dx < row.length; dx++) {
          const glyph = row[dx];
          if (glyph === '.') continue;
          const shade = glyph === 'C' ? contrast : glyph === 'I' ? 12 : glyph === 'L' ? 16 : color;
          add(left + dx, top + dy, shade);
        }
      });
    }
  }
  return [...events].map(([pixelId, shade]) => ({pixelId, color: shade}));
}

export function chunkEvents(events) {
  const batches = [];
  for (let start = 0; start < events.length; start += 64) batches.push(events.slice(start, start + 64));
  return batches;
}

export function kaleidoscopeFillBatches(cells, target, width, height) {
  if (width !== height) return [];
  const seen = new Set(), orbits = [];
  for (let y = 0; y < height; y++) for (let x = 0; x < width; x++) {
    const id = y * width + x;
    if (seen.has(id) || cells[id] !== 0 || target[id] <= 0) continue;
    const points = [[x,y], [width-1-x,y], [x,height-1-y], [width-1-x,height-1-y],
      [y,x], [width-1-y,x], [y,height-1-x], [width-1-y,height-1-x]];
    const orbit = [...new Set(points.map(([px,py]) => py * width + px))];
    orbit.forEach(point => seen.add(point));
    if (orbit.some(point => cells[point] !== 0 || target[point] !== target[id])) continue;
    const distance = (x-(width-1)/2)**2 + (y-(height-1)/2)**2;
    orbits.push({distance, events: orbit.map(pixelId => ({pixelId, color: target[pixelId]}))});
  }
  orbits.sort((a,b) => a.distance - b.distance);
  const batches = []; let batch = [];
  for (const orbit of orbits) {
    if (batch.length + orbit.events.length > 64) { batches.push(batch); batch = []; }
    batch.push(...orbit.events);
  }
  if (batch.length) batches.push(batch);
  return batches;
}
