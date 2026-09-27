import { cloudflareTest } from '@cloudflare/vitest-plugin';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  plugins: [cloudflareTest({ wrangler: { configPath: './cloudflare/worker/wrangler.jsonc' } })],
  test: { include: ['cloudflare/**/*.test.js'] },
});
