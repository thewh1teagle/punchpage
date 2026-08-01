import {defineConfig} from 'vite';

/**
 * Entries that must land at the dist root under an exact, unhashed filename:
 * the Go host injects `__punchpage_runtime__.js` by name and the app registers
 * `sw.js` as a classic service worker. Both are self-contained, so the emitted
 * ES output contains no import/export statements.
 */
const ROOT_ENTRIES: Record<string, string> = {
  sw: 'src/sw.ts',
  __punchpage_runtime__: 'src/punchpage-runtime.ts'
};

export default defineConfig({
  base: './',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    sourcemap: true,
    rollupOptions: {
      input: {index: 'index.html', ...ROOT_ENTRIES},
      output: {
        entryFileNames: chunk =>
          chunk.name in ROOT_ENTRIES ? '[name].js' : 'assets/[name]-[hash].js'
      }
    }
  }
});
