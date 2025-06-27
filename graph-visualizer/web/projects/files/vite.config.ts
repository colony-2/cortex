import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { resolve } from 'path';

export default defineConfig({
  plugins: [react()],
  build: {
    lib: {
      entry: resolve(__dirname, 'src/index.ts'),
      name: 'GraphVisualizerFiles',
      fileName: 'index',
      formats: ['es', 'cjs']
    },
    rollupOptions: {
      external: ['react', 'react-dom', 'antd', '@ant-design/icons', '@graph-visualizer/shared'],
      output: {
        globals: {
          react: 'React',
          'react-dom': 'ReactDOM',
          antd: 'antd',
          '@ant-design/icons': 'AntdIcons',
          '@graph-visualizer/shared': 'GraphVisualizerShared'
        }
      }
    }
  }
});