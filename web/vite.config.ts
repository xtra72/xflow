/// <reference types="vitest" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import { resolve } from 'path';

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': resolve(__dirname, './src'),
    },
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8081',
        changeOrigin: true,
      },
      '/ws': {
        target: 'http://127.0.0.1:8081',
        ws: true,
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
    css: false,
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'html'],
      include: [
        'src/services/ws/chartChannel.ts',
        'src/services/api/charts.ts',
        'src/pages/dashboard/panels/charts/**',
        'src/pages/dashboard/AddPanelDialog.tsx',
        'src/pages/dashboard/ChartPanelSections.tsx',
        'src/pages/dashboard/TsdbSourceSection.tsx',
        // SPEC-CHART-004 · SPEC-CHART-005 — 패널 요소 직접 편집. 목록이 명시
        // 허용목록이라 새 파일은 여기 적어야 측정된다(적지 않으면 조용히 0건으로
        // 빠진다). 005 가 통계 전용 모듈을 공용 모듈로 끌어올리며 이름이 Stat* →
        // Panel* 로 바뀌었으므로, 옛 이름을 남겨 두면 그 항목은 존재하지 않는
        // 파일을 가리켜 측정 대상이 조용히 비게 된다.
        'src/pages/dashboard/PanelDragLayer.tsx',
        'src/pages/dashboard/PanelAlignToolbar.tsx',
        'src/pages/dashboard/PanelEditGrid.tsx',
        'src/pages/dashboard/PanelGridBackdrop.tsx',
        'src/pages/dashboard/PanelResizeOverlay.tsx',
        'src/pages/dashboard/usePanelElementEdit.tsx',
        'src/pages/dashboard/panelEditContext.ts',
        'src/pages/dashboard/previewStage.ts',
        'src/pages/dashboard/previewGridSize.ts',
        'src/pages/dashboard/StatElementStylePopover.tsx',
        'src/pages/dashboard/textStyleFields.tsx',
        'src/services/api/seriesMatrixPivot.ts',
        'src/services/api/seriesDataSource.ts',
        'src/services/api/tsdbSource.ts',
      ],
      exclude: [
        '**/*.test.ts',
        '**/*.test.tsx',
        '**/index.ts',
      ],
    },
  },
});
