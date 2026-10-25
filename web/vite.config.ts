import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    target: 'es2022',
    assetsInlineLimit: 0, // no data: URIs; everything is served from 'self'
    chunkSizeWarningLimit: 400,
  },
  server: {
    proxy: {
      '/api': { target: 'http://127.0.0.1:18840', changeOrigin: false },
    },
  },
});
