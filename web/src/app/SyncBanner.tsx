import { Button } from '../ui/Button';
import { Card, Note } from '../ui/Card';

type Props = {
  offline: boolean;
  rejected: string[];
  onRetry: () => void;
  onClearRejected: () => void;
};

// 同期の異常だけを出す。正常なときは何も出さない。
//
// 以前は下端のバーが常に「同期済み」と言っていた。常設すると、本当に
// 困っているときの赤も同じ場所に出るので、目が慣れて気づかなくなる。
export function SyncBanner({ offline, rejected, onRetry, onClearRejected }: Props) {
  if (!offline && rejected.length === 0) return null;

  return (
    <>
      {offline && (
        <Card title="つながりません">
          <Note>
            今日のメニューはサーバーが組むので、圏外では出せません。古いメニューを
            キャッシュして出すことはしていません。前回の重量が今日の重量として
            表示されると、記録そのものが壊れるためです。
          </Note>
          <Button variant="quiet" className="mt-3" onClick={onRetry}>
            もう一度つなぐ
          </Button>
        </Card>
      )}

      {rejected.length > 0 && (
        <Card title="送れなかった記録">
          <Note>
            サーバーが受け付けなかったので、送るのをやめました。同じものを送り続けると、
            あとの記録がすべて詰まるためです。必要なら入れ直してください。
          </Note>
          <ul className="mt-2.5 list-disc pl-[1.2em] text-xs leading-[1.7] text-faint">
            {rejected.map((r, i) => (
              <li key={i}>{r}</li>
            ))}
          </ul>
          <Button variant="quiet" className="mt-3" onClick={onClearRejected}>
            消す
          </Button>
        </Card>
      )}
    </>
  );
}
