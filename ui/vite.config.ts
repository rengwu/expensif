import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { resolve } from 'path';

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: resolve(__dirname, '../static'),
    emptyOutDir: true,
    manifest: true,
    rollupOptions: {
      input: {
        categoryPills: resolve(__dirname, 'src/entries/category-pills.tsx'),
        descriptionPills: resolve(__dirname, 'src/entries/description-pills.tsx'),
        dataTable: resolve(__dirname, 'src/entries/data-table.tsx'),
        mobileNav: resolve(__dirname, 'src/entries/mobile-nav.tsx'),
        // Not a React island: the daily view's infinite scroll is plain TS.
        dailyScroll: resolve(__dirname, 'src/entries/daily-scroll.ts'),
        // Not a React island either: the Export page's live preview is plain TS
        // that fetches a server fragment and swaps it into place.
        exportPreview: resolve(__dirname, 'src/entries/export-preview.ts'),
      },
      output: {
        entryFileNames: 'js/[name].js',
        chunkFileNames: 'js/[name]-[hash].js',
        assetFileNames: (assetInfo) => {
          const info = assetInfo.name.split('.');
          const ext = info[info.length - 1];
          return `assets/[name]-[hash][extname]`;
        },
      },
    },
  },
  server: {
    port: 8081,
    strictPort: true,
    host: true,
    proxy: {
      '/api': 'http://localhost:8080',
      '^/.*': {
        target: 'http://localhost:8080',
        bypass(req) {
          // Let Vite serve its own internals and source files
          const url = req.url || '';
          if (url.startsWith('/@') || url.startsWith('/src/') || url.startsWith('/node_modules/')) {
            return url;
          }
        },
      },
    },
  },
});
