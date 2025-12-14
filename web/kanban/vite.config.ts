import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { resolve } from 'path';

export default defineConfig({
  plugins: [react()],
  build: {
    lib: {
      entry: 'src/index.ts',
      name: '@vibethis/kanban',
      formats: ['es', 'cjs'],
    },
    rollupOptions: {
      external: ['react', 'react-dom', 'antd', '@vibethis/shared', '@vibethis/openapi-client'],
    },
  },
  resolve: {
    alias: {
      '@vibethis/shared': resolve(__dirname, '../shared/src/index.ts'),
    },
    dedupe: ['react', 'react-dom'],
  },
});
