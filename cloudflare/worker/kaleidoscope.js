const WIDTH = 200;
const randomInt = (min, max) => min + Math.floor(Math.random() * (max - min + 1));

function schemeExcept(previous = []) {
  let scheme;
  do {
    scheme = [...Array(16)].map((_, index) => index + 1).sort(() => Math.random() - 0.5).slice(0, 4);
  } while (scheme.every((value, index) => value === previous[index]));
  return scheme;
}

export function randomKaleidoscope(previous = []) {
  let cells;
  let scheme;
  do {
    scheme = schemeExcept();
    const inner = randomInt(12, 24);
    const ring = randomInt(42, 63);
    const ringWidth = randomInt(9, 19);
    const outer = randomInt(76, 91);
    const spokes = randomInt(3, 7);
    const phase = Math.random() * Math.PI;
    cells = new Array(WIDTH * WIDTH);
    for (let y = 0; y < WIDTH; y++) {
      for (let x = 0; x < WIDTH; x++) {
        const dx = Math.abs(x - 99.5);
        const dy = Math.abs(y - 99.5);
        const radius = Math.hypot(dx, dy);
        const angle = Math.atan2(Math.min(dx, dy), Math.max(dx, dy));
        const wave = Math.abs(Math.sin(spokes * angle + phase));
        const inside = radius < inner || Math.abs(radius - ring) < ringWidth * (0.4 + wave) || Math.abs(radius - outer) < 4 + 4 * wave;
        const band = Math.floor(radius / (19 + spokes));
        const wedge = Math.floor(angle * (5 + spokes) / Math.PI);
        cells[y * WIDTH + x] = inside ? scheme[(band + wedge) % scheme.length] : -1;
      }
    }
  } while (previous.length === cells.length && cells.every((value, index) => (value === -1) === (previous[index] === -1)));
  return { cells, scheme };
}

export function recolorKaleidoscope(previous, oldScheme = []) {
  const scheme = schemeExcept(oldScheme);
  const bandSize = randomInt(12, 32);
  const wedgeCount = randomInt(4, 14);
  const phase = randomInt(0, 3);
  const cells = new Array(previous.length);
  const target = new Array(previous.length);
  for (let y = 0; y < WIDTH; y++) {
    for (let x = 0; x < WIDTH; x++) {
      const id = y * WIDTH + x;
      const dx = Math.abs(x - 99.5);
      const dy = Math.abs(y - 99.5);
      const radius = Math.hypot(dx, dy);
      const angle = Math.atan2(Math.min(dx, dy), Math.max(dx, dy));
      const band = Math.floor(radius / bandSize);
      const wedge = Math.floor(angle * wedgeCount / Math.PI);
      cells[id] = previous[id] === -1 ? -1 : 0;
      target[id] = scheme[(band + wedge + phase) % scheme.length];
    }
  }
  return { cells, target, scheme };
}
