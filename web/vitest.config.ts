import { defineConfig } from 'vite';

export default defineConfig({
  test: {
    exclude: [
      '**/node_modules/**',
      '**/dist/**',
      '**/.moon/**',
      '**/tests/**', // Exclude e2e/playwright tests
      '**/playwright/**',
      '**/*.spec.ts' // Exclude playwright spec files
    ]
  }
});