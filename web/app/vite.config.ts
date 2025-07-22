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
    chunkSizeWarningLimit: 1500,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules')) {
            if (id.includes('react') || id.includes('react-dom') || id.includes('react-router')) {
              return 'react-vendor';
            }
            if (id.includes('antd') || id.includes('@ant-design')) {
              return 'antd';
            }
            if (id.includes('monaco-editor')) {
              return 'monaco';
            }
            if (id.includes('@xyflow')) {
              return 'xyflow';
            }
            if (id.includes('@rjsf')) {
              return 'rjsf';
            }
            if (id.includes('js-yaml')) {
              return 'yaml';
            }
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
    }
  },
  test: {
    exclude: ['tests/**', 'node_modules/**', 'dist/**', '.moon/**']
  }
});