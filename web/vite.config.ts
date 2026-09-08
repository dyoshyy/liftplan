import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import { VitePWA } from 'vite-plugin-pwa';

// 開発サーバーは proxy を置かない。
//
// /api を proxy で手元に見せると、ブラウザから見て同一オリジンになり、
// CORS を一度も通らないまま開発が終わる。設定漏れが本番で初めて出る。
// 本番と同じくクロスオリジンで叩き、手元で落ちるようにする。
export default defineConfig(({ command, mode }) => {
  // 実行時に落ちるより、ビルドで止まるほうが早く気づく。
  // 出してから「記録が消える」に気づくのが最悪。
  //
  // process.env ではなく loadEnv を見る。Vite は .env.local からも読むので、
  // process.env だけを見ると、手元で正しく設定してあるのに落ちる。
  if (command === 'build' && !loadEnv(mode, process.cwd(), '').VITE_API_BASE) {
    throw new Error('VITE_API_BASE が設定されていない。API のオリジンを指定してビルドすること');
  }

  return {
    plugins: [
      react(),
      tailwindcss(),
      VitePWA({
        // Service Worker は手書きのまま保つ。injectManifest は
        // ハッシュ付き資産の一覧を差し込むだけで、中身の規則は触らない。
        strategies: 'injectManifest',
        srcDir: 'src',
        filename: 'sw.ts',
        injectRegister: null,
        registerType: 'prompt',
        manifest: false,
        injectManifest: {
          globPatterns: ['**/*.{js,css,html,svg,webmanifest}'],
        },
        devOptions: { enabled: false },
      }),
    ],
    build: {
      // 資産のハッシュはそのまま。index.html だけが更新の起点になる。
      sourcemap: true,
    },
    test: {
      environment: 'node',
      include: ['src/**/*.test.ts'],
    },
  };
});
