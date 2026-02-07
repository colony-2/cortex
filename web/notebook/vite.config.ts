import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { resolve } from 'path';

export default defineConfig({
  plugins: [react()],
  build: {
    lib: {
      entry: 'src/index.ts',
      name: '@colony2/notebook',
      formats: ['es', 'cjs'],
    },
    rollupOptions: {
      external: ['react', 'react-dom', 'antd', '@ant-design/icons', 'dayjs', '@colony2/openapi-client'],
    },
  },
  resolve: {
    alias: {
      '@colony2/openapi-client': resolve(__dirname, '../openapi/src/index.ts'),
    },
    dedupe: ['react', 'react-dom'],
  },
});
