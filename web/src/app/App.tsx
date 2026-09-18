import { useCallback, useEffect, useRef, useState } from 'react';
import { getToken } from '../storage/local';
import { label, today } from '../domain/date';
import { Setup } from '../features/setup/Setup';
import { GearIcon } from '../ui/icons';
import { Today } from '../features/today/Today';
import { History } from '../features/history/History';
import { useStats } from '../features/history/useStats';
import { SettingsScreen } from '../features/settings/SettingsScreen';
import { RestTimerBar } from '../features/timer/RestTimerBar';
import { useRestTimer } from '../features/timer/useRestTimer';
import { BottomNav } from './BottomNav';
import { SyncBanner } from './SyncBanner';
import { useRoute } from './useRoute';
import { useLiftplan } from './useLiftplan';
import { useOutbox } from './useOutbox';

// 画面は1つ。履歴と設定は落とした（D-120）。
//
// 目的はジムで1回のセッションを記録し終えること。記録は溜まり続けるので、
// 見たくなったときに履歴を戻せばよい。
export function App() {
  const { data, status, setStatus, loadAll, recordLocally, forgetLocally } = useLiftplan();
  const outbox = useOutbox(useCallback((id: string) => data.names.get(id) ?? id, [data.names]));
  const { flush } = outbox;

  const timer = useRestTimer();
  const { route, go } = useRoute();


  const [online, setOnline] = useState(navigator.onLine);
  const [hasToken, setHasToken] = useState(() => getToken() !== '');
  // 履歴は開いたときだけ読む。毎回の読み込みに混ぜると、ジムで開くたびに
  // 見ないものを取りに行くことになる。
  const stats = useStats(hasToken && route === 'history');

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

  const nameOf = (id: string) => data.names.get(id) ?? id;

  return (
    <>
      <header className="sticky top-0 z-30 border-b border-line-soft bg-ground/90 backdrop-blur-[10px]">
        <div className="flex items-center gap-3 px-4 pb-3.5 pt-3.5">
          <div className="num text-[19px] font-semibold uppercase tracking-[0.08em]">
            lift<span className="text-amber">plan</span>
          </div>
          <div className="ml-auto text-[13px] text-muted">{label(today())}</div>
          {hasToken && (
            <button
              type="button"
              aria-label={route === 'settings' ? '設定を閉じる' : '設定'}
              aria-pressed={route === 'settings'}
              onClick={() => go(route === 'settings' ? 'today' : 'settings')}
              className={`-mr-1 p-1 ${route === 'settings' ? 'text-amber' : 'text-muted'}`}
            >
              <GearIcon />
            </button>
          )}
        </div>
      </header>

      <main className="mx-auto grid max-w-[620px] gap-3.5 p-4">
        {!hasToken ? (
          <Setup
            pending={outbox.pending}
            onSaved={() => {
              setHasToken(true);
              setStatus('loading');
            }}
          />
        ) : (
          <>
            {/* 同期の異常はどの画面にいても出す。記録が送れていないことは、
                いま何を見ているかと関係なく知らせる必要がある。 */}
            <SyncBanner
              offline={status === 'offline'}
              rejected={outbox.rejected}
              onRetry={() => void reload()}
              onClearRejected={() => void outbox.clearRejected()}
            />

            {route === 'today' && (
              <Today
                data={data}
                enqueue={outbox.enqueue}
                onRecordLocally={recordLocally}
                onForgetLocally={forgetLocally}
                onRecorded={timer.start}
                canStartRest={timer.state.kind === 'idle'}
                onReload={reload}
              />
            )}

            {route === 'history' && (
              <History
                stats={stats.stats}
                days={data.days}
                error={stats.error}
                onReload={() => void stats.reload()}
              />
            )}

            {route === 'settings' && (
              <SettingsScreen
                nameOf={nameOf}
                allExerciseIds={[...data.names.keys()]}
                onChanged={reload}
                timer={timer}
                onForget={() => {
                  setHasToken(false);
                  go('today');
                }}
              />
            )}
          </>
        )}
      </main>

      {hasToken && <RestTimerBar timer={timer} />}

      {hasToken && (
        <BottomNav
          route={route}
          onGo={go}
          pending={outbox.pending}
          rejected={outbox.rejected.length}
          online={online}
          onSync={() => void reload()}
        />
      )}
    </>
  );
}
