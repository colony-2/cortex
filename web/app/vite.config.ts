import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { resolve } from 'path';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    open: true
  },
  build: {
    outDir: 'dist',
    sourcemap: true
  },
  resolve: {
    alias: {
      '@vibethis/shared': resolve(__dirname, '../shared/src/index.ts'),
      '@vibethis/changes': resolve(__dirname, '../changes/src/index.ts'),
      '@vibethis/config': resolve(__dirname, '../config/src/index.ts'),
      '@vibethis/files': resolve(__dirname, '../files/src/index.ts'),
      '@vibethis/flowchart': resolve(__dirname, '../flowchart/src/index.ts')
    }
  },
  test: {
    exclude: ['tests/**', 'node_modules/**', 'dist/**', '.moon/**']
  }
});