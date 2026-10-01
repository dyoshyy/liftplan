import { useCallback } from 'react';
import type { DayChange } from '../domain/days';
import { label, today } from '../domain/date';
import { ExerciseManager } from '../features/exercises/ExerciseManager';
import { Forecast } from '../features/forecast/Forecast';
import { Setup } from '../features/setup/Setup';
import { GearIcon } from '../ui/icons';
import { Today } from '../features/today/Today';
import { History } from '../features/history/History';
import { useHistoryEditor } from '../features/history/useHistoryEditor';
import { useMonthLogs } from '../features/history/useMonthLogs';
import { useStats } from '../features/history/useStats';
import { SettingsScreen } from '../features/settings/SettingsScreen';
import { RestTimerBar } from '../features/timer/RestTimerBar';
import { useRestTimer } from '../features/timer/useRestTimer';
import { BottomNav } from './BottomNav';
import { SyncBanner } from './SyncBanner';
import { UpdateBanner } from './UpdateBanner';
import { useRoute } from './useRoute';
import { useSessionOrchestrator } from './useSessionOrchestrator';

// 画面の骨組みと、どの画面を出すかの選択だけを持つ。
//
// 調停（読み込みと待ち行列の順序、認証、電波）は useSessionOrchestrator、
// 記録の手順は useRecordOrchestrator にある。ここに書くと、DOM を立てない
// と検査できないものに戻る。
export function App() {
  const { route, go } = useRoute();
  const timer = useRestTimer();

  // 調停は useSessionOrchestrator が持つ。この部品は描画と、
  // どの画面を出すかの選択だけをする。
  const session = useSessionOrchestrator();
  const { hasToken, load, online, data, outbox, reload, nameOf } = session;

  // 履歴は開いたときだけ読む。毎回の読み込みに混ぜると、ジムで開くたびに
  // 見ないものを取りに行くことになる。
  const stats = useStats(hasToken && route === 'history');
  const monthLogs = useMonthLogs(hasToken && route === 'history', data.days);
  // 履歴での修正は、直近の記録（今月と今日の画面）と、取ってある過去の月の
  // 両方に当てる。片方だけだと、月を行き来したときに直す前の値に戻って見える。
  const { applyLocally } = session;
  const { patch: patchMonth } = monthLogs;
  const onHistoryEdited = useCallback(
    (change: DayChange) => {
      applyLocally(change);
      patchMonth(change);
    },
    [applyLocally, patchMonth],
  );
  const editor = useHistoryEditor({ enqueue: outbox.enqueue, onApplied: onHistoryEdited });

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
              // 44px は指で押す的の下限。アイコンは 20px のままで、
              // 押せる範囲だけ広げる（見た目を大きくすると主張が強くなる）。
              className={`-mr-2 grid size-11 place-content-center ${
                route === 'settings' ? 'text-amber' : 'text-muted'
              }`}
            >
              <GearIcon />
            </button>
          )}
        </div>
      </header>

      <main className="mx-auto grid max-w-[620px] gap-3.5 p-4">
        {/* 更新の通知は認証の外に置く。トークンを入れる前でも出す必要がある。
            古いバンドルのせいでログインできない状態になりうるので、そこで
            脱出口が消えていると、画面から抜ける手段が無くなる。 */}
        <UpdateBanner />

        {!hasToken ? (
          <Setup pending={outbox.pending} />
        ) : (
          <>
            {/* 同期の異常はどの画面にいても出す。記録が送れていないことは、
                いま何を見ているかと関係なく知らせる必要がある。 */}
            <SyncBanner
              offline={load === 'offline'}
              rejected={outbox.rejected}
              onRetry={() => void reload()}
              onClearRejected={() => void outbox.clearRejected()}
            />

            {route === 'today' && (
              <Today
                data={data}
                enqueue={outbox.enqueue}
                onRecordLocally={session.recordLocally}
                onForgetLocally={session.forgetLocally}
                onRecorded={timer.start}
                canStartRest={timer.state.kind === 'idle'}
                onReload={reload}
                onOpenForecast={() => go('forecast')}
              />
            )}

            {route === 'history' && (
              <History
                logs={monthLogs}
                editor={editor}
                stats={stats.stats}
                statsError={stats.error}
                onReloadStats={() => void stats.reload()}
              />
            )}

            {route === 'settings' && (
              <SettingsScreen
                nameOf={nameOf}
                exercises={data.exercises}
                onChanged={reload}
                timer={timer}
                onForget={() => {
                  session.signOut();
                  go('today');
                }}
                onOpenExercises={() => go('exercises')}
              />
            )}

            {route === 'forecast' && (
              <Forecast nameOf={nameOf} onBack={() => go('today')} />
            )}

            {route === 'exercises' && (
              <ExerciseManager exercises={data.exercises} onChanged={reload} onBack={() => go('settings')} />
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
