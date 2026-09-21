import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { DevSimulation } from './DevSimulation';
import '../styles/index.css';

// Service Worker は登録しない。開発用の画面を precache させると、
// 直したのに古い版が出続ける。本番の登録は src/main.tsx にある。

const root = document.getElementById('root');
if (!root) throw new Error('#root が無い');

createRoot(root).render(
  <StrictMode>
    <DevSimulation />
  </StrictMode>,
);
