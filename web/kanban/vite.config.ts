import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { resolve } from 'path';

export default defineConfig({
  plugins: [react()],
  build: {
    lib: {
      entry: 'src/index.ts',
      name: '@colony2/kanban',
      formats: ['es', 'cjs'],
    },
    rollupOptions: {
      external: ['react', 'react-dom', 'antd', '@colony2/shared', '@colony2/openapi-client'],
    },
  },
  resolve: {
    alias: {
      '@colony2/shared': resolve(__dirname, '../shared/src/index.ts'),
    },
    dedupe: ['react', 'react-dom'],
  },
});
