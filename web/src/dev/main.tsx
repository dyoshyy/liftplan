import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { DevSimulation } from './DevSimulation';
import '../styles/index.css';

// Service Worker は登録しない。この画面は圏外で開く理由が無く、
// precache に入れると更新の合図を押すまで古い版が出続ける。
// 登録は src/main.tsx（メインの画面）にある。

const root = document.getElementById('root');
if (!root) throw new Error('#root が無い');

createRoot(root).render(
  <StrictMode>
    <DevSimulation />
  </StrictMode>,
);
