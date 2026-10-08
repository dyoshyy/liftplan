import { useSyncExternalStore } from 'react';
import { Button } from '../ui/Button';
import { Card, Note } from '../ui/Card';
import { applyUpdate, hasUpdate, subscribeToUpdate } from './swUpdate';

// 新しいバージョンが待っていることを知らせる。
//
// 押すまで切り替えない。記録の途中で画面が入れ替わるほうが害が大きい。
// ただし押せる場所が無いと、古いバージョンのまま取り残される端末ができる。
//
// 共通の Card / Button を使う。以前は index.css の .card / .btn を直に
// 書いていて、あちらを消したときに**枠も角丸もボタンの見た目も無い
// 四角が出るだけ**になっていた。
export function UpdateBanner() {
  const needRefresh = useSyncExternalStore(subscribeToUpdate, hasUpdate, () => false);

  if (!needRefresh) return null;

  return (
    <Card tone="amber" className="animate-drop-in">
      {/* Card の title は薄い灰色で、注意を引くカードには弱い。
          本文の1行目として普通の明るさで出す。 */}
      <p className="mb-1 font-bold">新しいバージョンがあります</p>
      <Note className="mb-3">記録の途中なら、終わってから押してください。押すまで今の画面のままです。</Note>
      {/* ボタンは quiet。カード自体が黄で主張しているので、塗りまで黄にすると
          強調が2箇所に割れる。黄は「その画面で一番やりたいこと」1つに使う。 */}
      <Button variant="quiet" onClick={() => void applyUpdate()}>
        新しいバージョンにする
      </Button>
    </Card>
  );
}
