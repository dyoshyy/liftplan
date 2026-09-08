import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { registerSW } from 'virtual:pwa-register';
import { App } from './app/App';
import './styles/index.css';

// Service Worker はここで登録する。
//
// 画面の中の部品から登録すると、**その部品が描かれるまで登録されない**。
// 以前は更新通知のコンポーネントが唯一の登録箇所で、そちらはトークンを
// 入れたあとにしか描かれなかったので、初回訪問では登録されなかった
// （ホーム画面に追加できない状態のまま気づけない）。
//
// 自動で差し替えない。skipWaiting を呼ばないので、待機中の新版は
// 全てのクライアントが閉じたときに有効になる。記録シートを開いている
// 最中に画面が入れ替わるより、新版が遅れるほうがよい。
registerSW({ immediate: true });

const root = document.getElementById('root');
if (!root) throw new Error('#root が無い');

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
