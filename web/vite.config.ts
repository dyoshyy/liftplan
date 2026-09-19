import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import { VitePWA } from 'vite-plugin-pwa';

// 開発サーバーは proxy を置かない。
//
// /api を proxy で手元に見せると、ブラウザから見て同一オリジンになり、
// CORS を一度も通らないまま開発が終わる。設定漏れが本番で初めて出る。
// 本番と同じくクロスオリジンで叩き、手元で落ちるようにする。
/** isLocal はその URL が開発機のものかを見る。配ってはいけない値の判定に使う。 */
const isLocal = (url: string): boolean =>
  /^https?:\/\/(localhost|127\.0\.0\.1|0\.0\.0\.0|\[::1\])(:|\/|$)/i.test(url.trim());

export default defineConfig(({ command, mode }) => {
  // 実行時に落ちるより、ビルドで止まるほうが早く気づく。
  // 出してから「記録が消える」に気づくのが最悪。
  //
  // process.env ではなく loadEnv を見る。Vite は .env.local からも読むので、
  // process.env だけを見ると、手元で正しく設定してあるのに落ちる。
  if (command === 'build') {
    const apiBase = loadEnv(mode, process.cwd(), '').VITE_API_BASE;
    if (!apiBase) {
      throw new Error('VITE_API_BASE が設定されていない。API のオリジンを指定してビルドすること');
    }

    // 手元を指したままのバンドルを出さない。
    //
    // **一度これで本番が壊れた。**.env.local に開発用の localhost:8080 が
    // 入っているので、値を渡さずにビルドすると黙って localhost 入りの
    // バンドルが出来上がる。それを配ると、利用者の画面は自分の端末の
    // 8080 を叩き、何も返らない。しかも Service Worker が掴むので、
    // 直した版を出しても開いたままの端末は古いほうを実行し続ける。
    //
    // preview で手元の動きを確かめたいときだけ ALLOW_LOCAL_API=1 で通す。
    // 開発サーバー（command === 'serve'）はここを通らないので影響しない。
    if (isLocal(apiBase) && !process.env.ALLOW_LOCAL_API) {
      throw new Error(
        `VITE_API_BASE が手元を指している: ${apiBase}\n` +
          '配ると利用者の端末の同じポートを叩きに行く。出すなら本番の URL を渡すこと。\n' +
          '手元で確かめるだけなら ALLOW_LOCAL_API=1 を付ける。',
      );
    }
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
