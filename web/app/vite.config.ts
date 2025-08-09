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
            // Monaco editor should be separate due to size
            if (id.includes('monaco-editor')) {
              return 'monaco';
            }
            // Everything else in vendor chunk
            return 'vendor';
          }
        }
      }
    }
  },
  resolve: {
    alias: {
      '@vibethis/shared': resolve(__dirname, '../shared/src/index.ts'),
      '@vibethis/changes': resolve(__dirname, '../changes/src/index.ts'),
      '@vibethis/config': resolve(__dirname, '../config/src/index.ts'),
      '@vibethis/files': resolve(__dirname, '../files/src/index.ts'),
      '@vibethis/flowchart': resolve(__dirname, '../flowchart/src/index.ts')
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