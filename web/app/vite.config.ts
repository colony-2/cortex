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
    sourcemap: true,
    chunkSizeWarningLimit: 5000,
    rollupOptions: {
      output: {
        // Simpler chunking strategy - put all vendors in one chunk to avoid load order issues
        manualChunks(id) {
          if (id.includes('node_modules')) {
            // Everything else in vendor chunk
            return 'vendor';
          }
        }
      }
    }
  },
  resolve: {
    alias: {
      '@colony2/shared': resolve(__dirname, '../shared/src')
    },
    dedupe: ['react', 'react-dom', '@ant-design/icons', 'antd']
  },
  optimizeDeps: {
    include: ['@ant-design/icons', 'antd']
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    exclude: ['tests/**', 'node_modules/**', 'dist/**', '.moon/**']
  }
});
