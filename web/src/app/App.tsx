import { useCallback, useState } from 'react';
import type { DayChange } from '../domain/days';
import { label, today } from '../domain/date';
import { ExerciseManager } from '../features/exercises/ExerciseManager';
import { Forecast } from '../features/forecast/Forecast';
import { Groups } from '../features/groups/Groups';
import { JoinInvite } from '../features/groups/JoinInvite';
import { usePendingInvite } from '../features/groups/usePendingInvite';
import { Note } from '../ui/Card';
import { CoachMarks } from '../features/onboarding/CoachMarks';
import { Onboarding } from '../features/onboarding/Onboarding';
import { Setup } from '../features/setup/Setup';
import { GearIcon } from '../ui/icons';
import { Today } from '../features/today/Today';
import { History } from '../features/history/History';
import { TodaySkeleton } from '../features/today/TodaySkeleton';
import { awaitingFirstLoad } from './firstLoad';
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
  const firstLoad = awaitingFirstLoad(load, data.session);
  // 初回の案内を済ませていなければ、どの画面よりも先に通い方を決めてもらう。
  // 歯車と下のナビを出さないのは、決める前に抜けると既定の通い方のまま
  // 「今日」に着地し、案内も次に開くまで出ないため。
  const onboarding = hasToken && !firstLoad && !data.onboarded;
  // 通い方を決めた直後にだけ、「今日」の使い方を指して見せる。メモリだけに
  // 持つので、再読み込みすれば出ない（済んだことは保存で記録済み）。
  const [coaching, setCoaching] = useState(false);
  // 「今日」へ戻してから流す。招待から入った人は、設定の途中で参加すると
  // 仲間の画面へ移っていて、そのままだとコーチマークの出番が来ない。
  const finishOnboarding = useCallback(async () => {
    await reload();
    go('today');
    setCoaching(true);
  }, [reload, go]);
  const stopCoaching = useCallback(() => setCoaching(false), []);
  const monthLogs = useMonthLogs(hasToken && route === 'history', data.days, !firstLoad);
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

  // 招待から入ったら仲間の画面へ。key を替えて一覧を読み直させる
  // （既に仲間の画面にいると、開き直さないかぎり読み直さない）。
  const [groupsKey, setGroupsKey] = useState(0);
  const invite = usePendingInvite(hasToken, () => {
    setGroupsKey((k) => k + 1);
    go('groups');
  });

  return (
    <>
      <header className="sticky top-0 z-30 border-b border-line-soft bg-ground/90 backdrop-blur-[10px]">
        <div className="flex items-center gap-3 px-4 pb-3.5 pt-3.5">
          <div className="num text-[19px] font-semibold uppercase tracking-[0.08em]">
            lift<span className="text-amber">plan</span>
          </div>
          <div className="ml-auto text-[13px] text-muted">{label(today())}</div>
          {hasToken && !onboarding && (
            <button
              type="button"
              data-coach="settings"
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
          <>
            {invite.pending && <Note>招待リンクを開きました。ログインすると参加できます。</Note>}
            <Setup pending={outbox.pending} />
          </>
        ) : (
          <>
            {/* 同期の異常はどの画面にいても出す。記録が送れていないことは、
                いま何を見ているかと関係なく知らせる必要がある。 */}
            <SyncBanner
              offline={load === 'offline'}
              rejected={outbox.rejected}
              failed={outbox.failed}
              onRetry={() => void reload()}
              onClearRejected={() => void outbox.clearRejected()}
              onClearFailed={outbox.clearFailed}
            />

            <JoinInvite invite={invite} />

            {/* 画面を替えたときだけふわっと出す。key が route なので、同じ画面の
                描き直しでは入り直さない。 */}
            {onboarding ? (
              <Onboarding exercises={data.exercises} onDone={finishOnboarding} />
            ) : (
              <div key={route} className="grid animate-fade-in gap-3.5">
                {route === 'today' && firstLoad && <TodaySkeleton />}

                {route === 'today' && !firstLoad && (
                  <Today
                    data={data}
                    enqueue={outbox.enqueue}
                    enqueueAll={outbox.enqueueAll}
                    onSaveFailed={outbox.reportFailure}
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
                    exercises={data.exercises}
                    selected={data.selected}
                    loadOffsets={data.loadOffsets}
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

                {route === 'forecast' && <Forecast nameOf={nameOf} onBack={() => go('today')} />}

                {route === 'groups' && <Groups key={groupsKey} />}

                {route === 'exercises' && (
                  <ExerciseManager
                    exercises={data.exercises}
                    onChanged={reload}
                    onBack={() => go('settings')}
                  />
                )}
              </div>
            )}
          </>
        )}
      </main>

      {coaching && route === 'today' && !firstLoad && <CoachMarks onDone={stopCoaching} />}

      {hasToken && !onboarding && <RestTimerBar timer={timer} />}

      {hasToken && !onboarding && (
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
