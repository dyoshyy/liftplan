import { useState } from 'react';
import { clearToken } from '../../storage/local';
import { Button } from '../../ui/Button';
import { Card, Note } from '../../ui/Card';

/**
 * Device はこの端末の設定。いまはトークンを消すことだけ。
 *
 * D-120 が「設定タブを戻すとき」の条件に「いまはトークンを消す手段も
 * 無い」と書いていた、その手段。端末を手放すとき、トークンを変えたとき、
 * 別の端末で試すときに要る。
 *
 * **`confirm()` は使わない。**ページ全体が止まり、待ち行列の送信も
 * 止まる。取り消しの効かない操作は、その場で二段階にする。
 *
 * 置き場所はメニューの設定の隣。設定と名の付くものを2箇所に散らさない。
 */
export function Device({ onForget }: { onForget: () => void }) {
  const [confirming, setConfirming] = useState(false);

  if (!confirming) {
    return (
      <Button variant="danger" onClick={() => setConfirming(true)}>
        この端末からトークンを消す
      </Button>
    );
  }

  return (
    <Card title="この端末">
      <Note className="mb-3">
        この端末からトークンを消します。未送信の記録は消えません。トークンを
        入れ直せばそこから送られます。
      </Note>
      <Button
        variant="danger"
        onClick={() => {
          clearToken();
          onForget();
        }}
      >
        消す
      </Button>
      <Button variant="quiet" className="mt-2" onClick={() => setConfirming(false)}>
        やめる
      </Button>
    </Card>
  );
}
