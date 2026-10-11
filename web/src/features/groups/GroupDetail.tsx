import { useState } from 'react';
import { label } from '../../domain/date';
import { BackButton } from '../../ui/BackButton';
import { Button } from '../../ui/Button';
import { Card, Note } from '../../ui/Card';
import { Loading, SkeletonCard } from '../../ui/Skeleton';
import { exerciseLabel, inviteUrl, WEEKDAYS, weekLog, weekRows, type LogDay, type Row } from './groups';
import { useGroupWeek } from './useGroupWeek';

type Props = {
  id: string;
  onBack: () => void;
  /** onLeft は抜けたあと。一覧へ戻す。 */
  onLeft: () => void;
};

// GroupDetail は1つのグループの今週。
//
// 格子で「だれがやったか」を、その下の記録で「何をやったか」を見る。
// 見せるのは日と種目名まで（設計書「何のためか」）。
export function GroupDetail({ id, onBack, onLeft }: Props) {
  const g = useGroupWeek(id);

  return (
    <div className="grid gap-3.5">
      <BackButton label="グループ" onClick={onBack} />
      {g.week && <h1 className="text-[19px] font-semibold">{g.week.name}</h1>}

      {g.error && <Note>{g.error}</Note>}
      {!g.week && !g.error && (
        <Loading>
          <SkeletonCard rows={4} />
          <SkeletonCard rows={4} />
        </Loading>
      )}

      {g.week && (
        <>
          <WeekGrid rows={weekRows(g.week)} />
          <WeekLog days={weekLog(g.week)} />
          {g.note && <Note>{g.note}</Note>}
          <GroupActions
            inviteToken={g.week.invite_token}
            busy={g.busy}
            onRotate={g.rotate}
            onLeave={async () => {
              if (await g.leave()) onLeft();
            }}
          />
        </>
      )}
    </div>
  );
}

const dayAnchor = (date: string) => `group-day-${date}`;

// WeekGrid は「だれが、どの日にやったか」の格子。● を押すと下の記録のその日へ移る。
function WeekGrid({ rows }: { rows: Row[] }) {
  return (
    <Card title="今週">
      <table className="w-full table-fixed border-collapse text-center">
        <thead>
          <tr>
            <th className="w-[34%]" />
            {WEEKDAYS.map((d) => (
              <th key={d} className="pb-2 text-xs font-normal text-faint">
                {d}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r, row) => (
            <tr key={`${row}-${r.name}`}>
              <th scope="row" className="truncate py-1.5 pr-2 text-left text-[14px] font-medium">
                {r.name}
                {r.isMe && <span className="ml-1 text-xs font-normal text-faint">(自分)</span>}
              </th>
              {r.cells.map((c) =>
                c.exercises === null ? (
                  <td key={c.date} className="text-faint" aria-label={`${r.name} ${label(c.date)} 記録なし`}>
                    ·
                  </td>
                ) : (
                  <td key={c.date}>
                    <button
                      type="button"
                      aria-label={`${r.name} ${label(c.date)} の記録へ`}
                      onClick={() =>
                        document.getElementById(dayAnchor(c.date))?.scrollIntoView({ behavior: 'smooth' })
                      }
                      className="mx-auto grid size-9 place-content-center rounded-full text-[15px] text-amber"
                    >
                      ●
                    </button>
                  </td>
                ),
              )}
            </tr>
          ))}
        </tbody>
      </table>
    </Card>
  );
}

// WeekLog は「何をやったか」。新しい日から、日ごとに人と種目を縦に並べる。
// 押さなくても全部読めるようにする。
function WeekLog({ days }: { days: LogDay[] }) {
  return (
    <Card title="今週の記録">
      {days.length === 0 ? (
        <Note>今週はまだだれも記録していません。</Note>
      ) : (
        <div className="grid gap-5">
          {days.map((d) => (
            <section key={d.date} id={dayAnchor(d.date)} className="grid scroll-mt-20 gap-3">
              <h2 className="border-b border-line-soft pb-1.5 text-[13px] text-muted">{label(d.date)}</h2>
              {d.entries.map((e) => (
                <div key={e.name + String(e.isMe)} className="grid gap-1">
                  <p className="text-[15px] font-medium">
                    {e.name}
                    {e.isMe && <span className="ml-1 text-xs font-normal text-faint">(自分)</span>}
                  </p>
                  <ul className="grid gap-0.5 pl-3">
                    {e.exercises.map((x, i) => (
                      <li key={`${i}-${x}`} className="text-[15px] text-muted">
                        {exerciseLabel(x)}
                      </li>
                    ))}
                  </ul>
                </div>
              ))}
            </section>
          ))}
        </div>
      )}
    </Card>
  );
}

type Confirm = 'rotate' | 'leave' | null;

// GroupActions はこのグループへの操作。グループに属さない操作（作る）は一覧に置く。
function GroupActions({
  inviteToken,
  busy,
  onRotate,
  onLeave,
}: {
  inviteToken: string;
  busy: boolean;
  onRotate: () => Promise<void>;
  onLeave: () => Promise<void>;
}) {
  const [confirm, setConfirm] = useState<Confirm>(null);
  const [shared, setShared] = useState('');
  const url = inviteUrl(location.origin, inviteToken);

  // 共有シートがあればそれ、無ければクリップボード。どちらも使えなければ
  // リンクをそのまま見せて、手で写してもらう。
  const share = async () => {
    setShared('');
    try {
      if (navigator.share) {
        await navigator.share({ title: 'liftplan に招待', url });
        return;
      }
      await navigator.clipboard.writeText(url);
      setShared('リンクをコピーしました');
    } catch {
      setShared(url);
    }
  };

  return (
    <Card title="このグループ">
      <div className="grid gap-2.5">
        <Button variant="quiet" size="md" disabled={busy} onClick={() => void share()}>
          招待リンクを共有
        </Button>
        {shared && <Note className="break-all">{shared}</Note>}

        {confirm === 'rotate' ? (
          <ConfirmRow
            message="今のリンクは使えなくなります"
            action="作り直す"
            busy={busy}
            onConfirm={async () => {
              await onRotate();
              setConfirm(null);
              setShared('');
            }}
            onCancel={() => setConfirm(null)}
          />
        ) : (
          <Button variant="ghost" size="md" disabled={busy} onClick={() => setConfirm('rotate')}>
            リンクを作り直す
          </Button>
        )}

        {confirm === 'leave' ? (
          <ConfirmRow
            message="抜けると、お互いの記録が見えなくなります"
            action="抜ける"
            danger
            busy={busy}
            onConfirm={async () => {
              await onLeave();
              setConfirm(null);
            }}
            onCancel={() => setConfirm(null)}
          />
        ) : (
          <Button variant="ghost" size="md" disabled={busy} onClick={() => setConfirm('leave')}>
            グループを抜ける
          </Button>
        )}
      </div>
    </Card>
  );
}

// ConfirmRow は画面内の確認。window.confirm は使わない（ブラウザの
// ダイアログは PWA で見た目が浮き、検査スクリプトからも扱いにくい）。
function ConfirmRow({
  message,
  action,
  danger = false,
  busy,
  onConfirm,
  onCancel,
}: {
  message: string;
  action: string;
  danger?: boolean;
  busy: boolean;
  onConfirm: () => Promise<void>;
  onCancel: () => void;
}) {
  return (
    <div className="grid gap-2 rounded-xl border border-line p-3">
      <p className="text-[13px] text-muted">{message}</p>
      <div className="grid grid-cols-2 gap-2">
        <Button variant="quiet" size="md" disabled={busy} onClick={onCancel}>
          やめる
        </Button>
        <Button
          variant={danger ? 'danger' : 'primary'}
          size="md"
          disabled={busy}
          onClick={() => void onConfirm()}
        >
          {action}
        </Button>
      </div>
    </div>
  );
}
