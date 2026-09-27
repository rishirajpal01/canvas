import {buildToolEvents} from './board-tools.mjs';

export function freshRandom() {
  const seed = new Uint32Array(1);
  crypto.getRandomValues(seed);
  let value = seed[0];
  return () => {
    value += 0x6D2B79F5;
    let step = value;
    step = Math.imul(step ^ step >>> 15, step | 1);
    step ^= step + Math.imul(step ^ step >>> 7, step | 61);
    return ((step ^ step >>> 14) >>> 0) / 4294967296;
  };
}

function fillNightSky(width, height, paint, random) {
  const center = width * (.44 + random() * .12);
  const tilt = width * (.19 + random() * .16);
  const drift = random() * Math.PI * 2;
  const dither = [
    [0, 32, 8, 40, 2, 34, 10, 42], [48, 16, 56, 24, 50, 18, 58, 26],
    [12, 44, 4, 36, 14, 46, 6, 38], [60, 28, 52, 20, 62, 30, 54, 22],
    [3, 35, 11, 43, 1, 33, 9, 41], [51, 19, 59, 27, 49, 17, 57, 25],
    [15, 47, 7, 39, 13, 45, 5, 37], [63, 31, 55, 23, 61, 29, 53, 21],
  ];
  const ridge = new Array(width);
  for (let x = 0; x < width; x++) {
    const position = x / width;
    ridge[x] = Math.floor(height * (.87
      + .045 * Math.sin(position * Math.PI * 2 + drift)
      + .022 * Math.sin(position * Math.PI * 5 + drift * 1.7)
      + .008 * Math.sin(position * Math.PI * 21 + drift)));
  }
  for (let y = 0; y < height; y++) for (let x = 0; x < width; x++) {
    if (y >= ridge[x]) {
      paint(x, y, y < ridge[x] + 3 ? 3 : 12);
      continue;
    }
    const depth = y / height;
    const axis = center + (depth - .5) * tilt + Math.sin(depth * 8 + drift) * width * .035;
    const distance = (x - axis) / Math.max(3, width * (.14 + .04 * Math.sin(depth * 5 + drift)));
    const band = Math.exp(-distance * distance * 1.6);
    const threshold = (dither[y % 8][x % 8] + .5) / 64;
    let color = 12;
    if (depth > .36 && threshold < Math.min(1, (depth - .36) * 2)) color = 2;
    if (depth > .62 && threshold < (depth - .62) * 2.5) color = 4;
    if (depth > .76 && threshold < (depth - .76) * 3.6) color = 10;
    const cloud = band * (.42 + .11 * Math.sin(y * .13 + drift) + .09 * Math.sin(x * .12 + y * .08));
    if (threshold < cloud) color = depth < .43 ? 3 : depth < .67 ? 10 : 4;
    if (band > .55 && random() < .012) color = depth > .55 ? 7 : 10;
    paint(x, y, color);
    const starChance = .014 + band * .028;
    if (random() < starChance) {
      const sparkle = random();
      paint(x, y, sparkle < .7 ? 11 : sparkle < .88 ? 10 : 6);
      if (sparkle > .985 && x > 0 && x < width - 1 && y > 0 && y < ridge[x] - 2) {
        paint(x - 1, y, 10); paint(x + 1, y, 10);
        paint(x, y - 1, 10); paint(x, y + 1, 10);
        paint(x, y, 11);
      }
    }
  }
  const drawTree = (centerX, heightFraction, widthFraction) => {
    const base = ridge[Math.max(0, Math.min(width - 1, centerX))] + 3;
    const treeHeight = Math.max(5, Math.floor(height * heightFraction));
    const halfWidth = Math.max(2, Math.floor(width * widthFraction));
    for (let y = Math.max(0, base - treeHeight); y <= base; y++) {
      const down = (y - (base - treeHeight)) / treeHeight;
      const branch = .72 + .28 * ((y - base + treeHeight) % 5) / 4;
      const spread = Math.ceil(halfWidth * down * branch);
      for (let x = centerX - spread; x <= centerX + spread; x++) paint(x, y, 12);
    }
  };
  for (const side of [0, 1]) {
    for (let i = 0; i < 5; i++) {
      const edge = Math.floor(random() * width * .17);
      const x = side ? width - 1 - edge : edge;
      drawTree(x, .08 + random() * .16, .025 + random() * .035);
    }
  }
}

function fillDailyMosaic(width, height, paint, random) {
  const style = Math.floor(random() * 3);
  const choose = colors => colors[Math.floor(random() * colors.length)];
  const ellipse = (cx, cy, rx, ry, angle, color) => {
    const cosine = Math.cos(angle), sine = Math.sin(angle);
    const reach = Math.ceil(Math.max(rx, ry));
    for (let y = Math.floor(cy - reach); y <= cy + reach; y++) for (let x = Math.floor(cx - reach); x <= cx + reach; x++) {
      const dx = x - cx, dy = y - cy;
      const across = (dx * cosine + dy * sine) / rx;
      const along = (-dx * sine + dy * cosine) / ry;
      if (across * across + along * along <= 1) paint(x, y, color);
    }
  };
  if (style === 0) {
    const palette = [3, 9, 16, 5, 7, 10, 4, 8, 6];
    const phase = random() * Math.PI * 2;
    const stretch = 14 + random() * 14;
    const bend = 10 + random() * 20;
    const swirlX = width * (.35 + random() * .3), swirlY = height * (.35 + random() * .3);
    for (let y = 0; y < height; y++) for (let x = 0; x < width; x++) {
      const dx = x - swirlX, dy = y - swirlY;
      const angle = Math.atan2(dy, dx);
      const radius = Math.hypot(dx, dy);
      const warp = x + bend * Math.sin(y / stretch + phase) + 13 * Math.sin(angle * 3 + radius / 16 + phase);
      const flow = Math.sin(warp / stretch + Math.sin(y / (stretch * .7) - phase))
        + .65 * Math.sin((y + 18 * Math.sin(x / (stretch * 1.3))) / (stretch * .8) + phase);
      const index = Math.max(0, Math.min(palette.length - 1, Math.floor((flow + 1.65) / 3.3 * palette.length)));
      paint(x, y, palette[index]);
    }
    return;
  }
  for (let y = 0; y < height; y++) for (let x = 0; x < width; x++) paint(x, y, 12);
  if (style === 1) {
    const colors = [5, 6, 7, 8, 9, 10, 11, 16, 4];
    const step = Math.max(9, Math.round(Math.min(width, height) / 14));
    for (let gy = -step; gy < height + step; gy += step) for (let gx = -step; gx < width + step; gx += step) {
      const x = gx + (random() - .5) * step * .5;
      const y = gy + (random() - .5) * step * .5;
      ellipse(x, y, step * (.22 + random() * .12), step * (.42 + random() * .16), random() * Math.PI, choose(colors));
    }
    return;
  }
  const flowers = [5, 6, 7, 8, 10, 4];
  const stemColors = [1, 3, 9, 15, 16];
  const spacing = Math.max(24, Math.round(Math.min(width, height) / 5));
  for (let gy = -spacing; gy < height + spacing; gy += spacing) for (let gx = -spacing; gx < width + spacing; gx += spacing) {
    const cx = gx + spacing * (.5 + (random() - .5) * .55);
    const cy = gy + spacing * (.5 + (random() - .5) * .55);
    const stemColor = choose(stemColors);
    const sway = (random() - .5) * spacing * .7;
    for (let step = 0; step < spacing; step++) {
      const y = cy + step;
      const x = cx + sway * Math.sin(step / spacing * Math.PI);
      paint(Math.round(x), Math.round(y), stemColor);
      if (step === Math.floor(spacing * .48)) {
        ellipse(x - 7, y - 2, 8, 3, -.6, stemColor);
        ellipse(x + 7, y + 4, 8, 3, .6, stemColor);
      }
    }
    const petals = 6 + Math.floor(random() * 4);
    const radius = spacing * (.27 + random() * .1);
    const offset = random() * Math.PI * 2;
    const petalColor = choose(flowers);
    for (let petal = 0; petal < petals; petal++) {
      const angle = offset + petal * Math.PI * 2 / petals;
      ellipse(cx + Math.cos(angle) * radius * .68, cy + Math.sin(angle) * radius * .68,
        radius * .75, radius * .3, angle, petalColor);
    }
    ellipse(cx, cy, radius * .29, radius * .29, 0, choose([6, 9, 11]));
    for (let dot = 0; dot < 8; dot++) {
      paint(Math.round(cx + (random() - .5) * spacing * 1.3), Math.round(cy + (random() - .5) * spacing * 1.3), choose([6, 7, 10]));
    }
  }
}

export function generateArtwork(boardId, width, height, previous, prompt = '', random = Math.random) {
  const cells = previous.map(cell => cell === -1 ? -1 : 0);
  const choose = list => list[Math.floor(random() * list.length)];
  const colorSets = [[5, 6, 7, 9], [2, 4, 10, 11], [8, 5, 6, 16], [1, 3, 9, 15]];
  const isDaily = boardId === 'daily' || /^daily-\d{4}-\d{2}-\d{2}$/.test(boardId);
  const colors = isDaily ? null : choose(colorSets);
  function paint(x, y, color) {
    if (x >= 0 && y >= 0 && x < width && y < height && cells[y * width + x] !== -1) cells[y * width + x] = color;
  }
  function stamp(theme, tool, x, y, color, origin) {
    for (const event of buildToolEvents(theme, tool, x, y, color, width, height, origin)) {
      if (cells[event.pixelId] !== -1) cells[event.pixelId] = event.color;
    }
  }
  function stampScaled(theme, tool, x, y, color, scale) {
    for (const event of buildToolEvents(theme, tool, x, y, color, width, height)) {
      const px = event.pixelId % width, py = Math.floor(event.pixelId / width);
      const left = x + (px - x) * scale, top = y + (py - y) * scale;
      for (let dy = 0; dy < scale; dy++) for (let dx = 0; dx < scale; dx++) paint(left + dx, top + dy, event.color);
    }
  }
  const theme = isDaily ? 'daily' : boardId;
  if (theme === 'daily') {
    fillDailyMosaic(width, height, paint, random);
  } else if (theme === 'garden') {
    const sway = random() * Math.PI * 2, bend = (random() - .5) * width * .4;
    const pathAt = y => width * .5 + bend * Math.sin(y / height * Math.PI + sway);
    for (let y = 0; y < height; y++) for (let x = 0; x < width; x++) {
      const depth = y / height;
      const pathWidth = width * (.025 + depth * .22);
      const distance = Math.abs(x - pathAt(y));
      if (distance < pathWidth) {
        const brick = (Math.floor(x / 7) + Math.floor(y / 4)) % 6;
        paint(x, y, brick === 0 ? 14 : brick === 1 ? 5 : 11);
      } else {
        const shade = random();
        paint(x, y, shade < .2 ? 12 : shade < .46 ? 15 : shade < .79 ? 16 : 3);
      }
    }
    for (let i = 0; i < Math.max(35, Math.floor(width * height / 180)); i++) {
      const x = Math.floor(random() * width), y = Math.floor(random() * height);
      if (Math.abs(x - pathAt(y)) < width * (.04 + y / height * .23)) continue;
      stamp('garden', choose(['flower', 'flower', 'leaf', 'butterfly']), x, y, choose([4, 6, 7, 8, 10, 11, 16]));
    }
  } else if (theme === 'night-sky') {
    fillNightSky(width, height, paint, random);
  } else if (theme === 'tiny-town') {
    const bend = (random() - .5) * width * .33, skyLine = .53 + random() * .08;
    const roadAt = y => width * .5 + bend * Math.sin((y / height) * Math.PI * 1.4);
    for (let y = 0; y < height; y++) for (let x = 0; x < width; x++) {
      const wave = .06 * Math.sin(x / width * 9 + bend) + .025 * Math.sin(x / width * 19);
      const hill = height * (skyLine + wave);
      if (y < hill) {
        const mountain = height * (.24 + .13 * Math.abs(Math.sin(x / width * 7 + bend)));
        const foreground = height * (.37 + .1 * Math.abs(Math.sin(x / width * 10 - bend)));
        const cloud = Math.sin(x * .17 + y * .13) + Math.sin(x * .08 - y * .21);
        paint(x, y, y >= foreground ? 3 : y >= mountain ? 1 : cloud > 1.73 ? 11 : 9);
        continue;
      }
      const depth = Math.max(0, (y - hill) / (height - hill));
      const roadWidth = width * (.025 + depth * .34);
      const distance = Math.abs(x - roadAt(y));
      if (distance < roadWidth) {
        const dash = Math.floor((height - y) / Math.max(5, 5 + depth * 12)) % 3 === 1;
        paint(x, y, distance < Math.max(1, roadWidth * .07) && dash ? 11 : distance > roadWidth - 2 ? 11 : 12);
      } else paint(x, y, random() < .12 ? 16 : 15);
    }
    for (let i = 0; i < Math.max(11, Math.floor(width / 17)); i++) {
      const y = Math.floor(height * (.61 + random() * .31));
      const side = random() < .5 ? -1 : 1;
      const offset = width * (.11 + random() * .18) + (y / height) * width * .22;
      const x = Math.floor(roadAt(y) + side * offset);
      const tool = random() < .42 ? 'tree' : choose(['house', 'shop']);
      stampScaled('tiny-town', tool, x, y, choose([5, 6, 7, 8, 10, 16]), y > height * .75 ? 3 : 2);
    }
  } else {
    const centerX = random() * width, centerY = random() * height;
    const scale = 5 + random() * Math.max(6, Math.min(width, height) / 4);
    const tilt = 2 + Math.floor(random() * 6);
    for (let y = 0; y < height; y++) for (let x = 0; x < width; x++) {
      const radius = Math.hypot(x - centerX, y - centerY);
      const wave = Math.sin((x + y * tilt) / scale + radius / (scale * 2));
      if (wave > .05 || (radius < scale * 1.5 && random() > .12)) paint(x, y, colors[Math.floor((radius / scale + wave * 2 + 8) * 2) % colors.length]);
    }
  }
  return cells;
}
