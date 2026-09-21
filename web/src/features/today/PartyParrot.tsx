import type { CSSProperties } from 'react';

// パーティーパロット。
//
// **画像は足さない。**元ネタは GIF だが、バンドルに画像を1枚入れると
// Service Worker のプリキャッシュに乗り、更新のたびに配り直すものが増える。
// 形はパスで足りる（跳ねる動きと色替えが本体で、描き込みではない）。
//
// 色替えは親（.pr-parrot）の filter: hue-rotate が steps() で回す。
// GIF のコマ送りの感じを出すのに、連続ではなく段で変える。
export function PartyParrot({ style }: { style?: CSSProperties }) {
  return (
    <span className="pr-parrot" style={style} aria-hidden="true">
      <svg viewBox="0 0 64 64" width="100%" height="100%" role="presentation">
        {/* 体 */}
        <path
          d="M18 50c-3-8-2-19 4-26 5-6 13-9 20-7 6 2 9 7 9 12 0 7-4 12-9 16-4 3-8 5-12 5h-12z"
          fill="#7cd44b"
        />
        {/* 頭 */}
        <circle cx="38" cy="23" r="13" fill="#9be05f" />
        {/* とさか */}
        <path d="M34 11c1-5 5-8 9-8-2 3-2 6-1 8-3-1-6-1-8 0z" fill="#f2d24b" />
        {/* くちばし */}
        <path d="M50 22l10 4-10 6-3-5z" fill="#f2903a" />
        {/* 目 */}
        <circle cx="41" cy="21" r="3.4" fill="#12181d" />
        <circle cx="42.2" cy="19.8" r="1.2" fill="#ffffff" />
        {/* 足 */}
        <path d="M24 50h5l-1 6h-3z M33 50h5l-1 6h-3z" fill="#f2903a" />
      </svg>
    </span>
  );
}
