import { useCallback, useEffect, useState } from 'react';
import { getToken } from '../storage/local';
import { label, today } from '../domain/date';
import { Setup } from '../features/setup/Setup';
import { Today } from '../features/today/Today';
import { History } from '../features/history/History';
import { Settings } from '../features/settings/Settings';
import { StatusBar } from './StatusBar';
import { Tabs, type View } from './Tabs';
import { UpdatePrompt } from './UpdatePrompt';
import { useLiftplan } from './useLiftplan';
import { useOutbox } from './useOutbox';

export function App() {
  const { data, status, setStatus, loadAll, loadToday, recordLocally, forgetLocally } =
    useLiftplan();
  const outbox = useOutbox(useCallback((id: string) => data.names.get(id) ?? id, [data.names]));
  const { flush, refresh } = outbox;

  const [view, setView] = useState<View>('today');
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

  useEffect(() => {
    const onOnline = () => {
      setOnline(true);
      void flush();
    };
    const onOffline = () => setOnline(false);
    window.addEventListener('online', onOnline);
    window.addEventListener('offline', onOffline);
    return () => {
      window.removeEventListener('online', onOnline);
      window.removeEventListener('offline', onOffline);
    };
  }, [flush]);

  if (!hasToken) {
    return (
      <Shell tabs={null}>
        <Setup
          pending={outbox.pending}
          onSaved={() => {
            setHasToken(true);
            setStatus('loading');
          }}
        />
        <StatusBar
          pending={outbox.pending}
          rejected={outbox.rejected.length}
          online={online}
          onReload={() => void refresh()}
        />
      </Shell>
    );
  }

  return (
    <Shell tabs={<Tabs view={view} onChange={setView} />}>
      <UpdatePrompt />

      {view === 'today' && (
        <Today
          data={data}
          offline={status === 'offline'}
          rejected={outbox.rejected}
          enqueue={outbox.enqueue}
          onClearRejected={() => void outbox.clearRejected()}
          onRetry={() => void reload()}
          onRecordLocally={recordLocally}
          onForgetLocally={forgetLocally}
          onReloadToday={(accepted) => void loadToday(accepted)}
        />
      )}

      {view === 'history' && <History stats={data.stats} days={data.days} />}

      {view === 'settings' && (
        <Settings
          program={data.program}
          exercises={data.exercises}
          onSaved={() => void reload()}
          onForget={() => setHasToken(false)}
        />
      )}

      <StatusBar
        pending={outbox.pending}
        rejected={outbox.rejected.length}
        online={online}
        onReload={() => void reload()}
      />
    </Shell>
  );
}

function Shell({ tabs, children }: { tabs: React.ReactNode; children: React.ReactNode }) {
  return (
    <>
      <header className="sticky top-0 z-30 border-b border-line-soft bg-ground/90 backdrop-blur-[10px]">
        <div className="flex items-center gap-3 px-4 pb-2.5 pt-3.5">
          <div className="num text-[19px] font-semibold uppercase tracking-[0.08em]">
            lift<span className="text-amber">plan</span>
          </div>
          <div className="ml-auto text-[13px] text-muted">{label(today())}</div>
        </div>
        {tabs}
      </header>
      <main className="mx-auto grid max-w-[620px] gap-3.5 p-4">{children}</main>
    </>
  );
}
