import { useCallback, useEffect, useRef, useState } from 'react';
import { getToken } from '../storage/local';
import { label, today } from '../domain/date';
import { Setup } from '../features/setup/Setup';
import { Today } from '../features/today/Today';
import { StatusBar } from './StatusBar';
import { useLiftplan } from './useLiftplan';
import { useOutbox } from './useOutbox';

// 画面は1つ。履歴と設定は落とした（D-120）。
//
// 目的はジムで1回のセッションを記録し終えること。記録は溜まり続けるので、
// 見たくなったときに履歴を戻せばよい。
export function App() {
  const { data, status, setStatus, loadAll, recordLocally, forgetLocally } = useLiftplan();
  const outbox = useOutbox(useCallback((id: string) => data.names.get(id) ?? id, [data.names]));
  const { flush, refresh } = outbox;

  const [online, setOnline] = useState(navigator.onLine);
  const [hasToken, setHasToken] = useState(() => getToken() !== '');

  // 溜まっているものを先に送りきってから読む。
  //
  // 逆にすると、送信前の状態で描画してから送ることになり、記録したのに
  // 緑が消えて見える。オフラインで記録して復帰したときに必ず踏み、
  // 「消えた」と思ってもう一度記録して重複する。
  const reload = useCallback(async () => {
    if (!hasToken) return;
    await flush();
    await loadAll();
  }, [hasToken, flush, loadAll]);

  useEffect(() => {
    void reload();
  }, [reload]);

  // トークンが通らなくなったら設定に戻す。
  useEffect(() => {
    if (status === 'unauthorized') setHasToken(false);
  }, [status]);

  // 復帰したときの判断に使う。status を購読すると、状態が変わるたびに
  // イベントの登録し直しが起きる。
  const statusRef = useRef(status);
  statusRef.current = status;

  // 復帰したら送るだけでなく、メニューも取り直す。
  //
  // 圏外で開くと「つながりません」だけの画面になる。電波が戻っても
  // 送信しかしないと、**メニューは空のまま**で、利用者が「更新」を
  // 押すまで今日の内容が出ない。ジムに着いて開き、電波を掴んだところで
  // 何も出ないのは、壊れているのと区別がつかない。
  //
  // 取り直すのはメニューが無いときだけ。毎回取り直すと、記録の最中に
  // 一瞬電波が切れただけで画面が組み替わる。
  useEffect(() => {
    const onOnline = () => {
      setOnline(true);
      if (statusRef.current === 'offline') void reload();
      else void flush();
    };
    const onOffline = () => setOnline(false);
    window.addEventListener('online', onOnline);
    window.addEventListener('offline', onOffline);
    return () => {
      window.removeEventListener('online', onOnline);
      window.removeEventListener('offline', onOffline);
    };
  }, [flush, reload]);

  return (
    <>
      <header className="sticky top-0 z-30 border-b border-line-soft bg-ground/90 backdrop-blur-[10px]">
        <div className="flex items-center gap-3 px-4 pb-3.5 pt-3.5">
          <div className="num text-[19px] font-semibold uppercase tracking-[0.08em]">
            lift<span className="text-amber">plan</span>
          </div>
          <div className="ml-auto text-[13px] text-muted">{label(today())}</div>
        </div>
      </header>

      <main className="mx-auto grid max-w-[620px] gap-3.5 p-4">
        {hasToken ? (
          <Today
            data={data}
            offline={status === 'offline'}
            rejected={outbox.rejected}
            enqueue={outbox.enqueue}
            onClearRejected={() => void outbox.clearRejected()}
            onRetry={() => void reload()}
            onRecordLocally={recordLocally}
            onForgetLocally={forgetLocally}
            onReload={reload}
          />
        ) : (
          <Setup
            pending={outbox.pending}
            onSaved={() => {
              setHasToken(true);
              setStatus('loading');
            }}
          />
        )}
      </main>

      <StatusBar
        pending={outbox.pending}
        rejected={outbox.rejected.length}
        online={online}
        onReload={() => void (hasToken ? reload() : refresh())}
      />
    </>
  );
}
