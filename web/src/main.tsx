import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { App } from './app/App';
// 読み込むだけで Service Worker が登録される。画面の部品に置くと、
// その部品が描かれるまで登録されない（以前それで初回訪問が漏れた）。
import './app/swUpdate';
import './styles/index.css';

const root = document.getElementById('root');
if (!root) throw new Error('#root が無い');

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
