import { useState } from 'react';
import { label } from '../../domain/date';
import { Button } from '../../ui/Button';
import { Card, Note } from '../../ui/Card';
import { LabeledInput, Select } from '../../ui/Field';
import { Loading, SkeletonCard } from '../../ui/Skeleton';
import { cn } from '../../ui/cn';
import { exerciseLabel, inviteUrl, nameProblem, WEEKDAYS, weekRows, type Row } from './groups';
import { useGroups } from './useGroups';

// Groups はグループのメンバーの今週を見る画面。
//
// 見せるのは日と種目名まで（設計書「何のためか」）。目的は続ける動機で、
// 「自分だけ空いている」に気づければよい。
export function Groups() {
  const g = useGroups();
  const [creating, setCreating] = useState(false);

  if (g.error) return <Note>{g.error}</Note>;
  if (g.groups === null) {
    return (
      <Loading>
        <SkeletonCard rows={4} />
      </Loading>
    );
  }
  if (g.groups.length === 0 || creating) {
    return (
      <CreateGroup
        busy={g.busy}
        note={g.note}
        onCancel={g.groups.length > 0 ? () => setCreating(false) : undefined}
        onCreate={async (name, displayName) => {
          if (await g.create(name, displayName)) setCreating(false);
        }}
      />
    );
  }

  return (
    <div className="grid gap-3.5">
      {g.groups.length > 1 ? (
        <Select aria-label="グループ" value={g.selectedId ?? ''} onChange={(e) => g.select(e.target.value)}>
          {g.groups.map((x) => (
            <option key={x.id} value={x.id}>
              {x.name}
            </option>
          ))}
        </Select>
      ) : (
        <h1 className="text-[19px] font-semibold">{g.groups[0]?.name}</h1>
      )}

      {g.week === null ? (
        <Loading>
          <SkeletonCard rows={4} />
        </Loading>
      ) : (
        <WeekCard rows={weekRows(g.week)} />
      )}

      {g.note && <Note>{g.note}</Note>}

      {g.week && (
        <GroupActions
          inviteToken={g.week.invite_token}
          busy={g.busy}
          onRotate={g.rotate}
          onLeave={g.leave}
          onCreateAnother={() => setCreating(true)}
        />
      )}
    </div>
  );
}

type Selected = { row: number; col: number };

function WeekCard({ rows }: { rows: Row[] }) {
  const [selected, setSelected] = useState<Selected | null>(null);
  const cell = selected ? rows[selected.row]?.cells[selected.col] : undefined;

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
              {r.cells.map((c, col) =>
                c.exercises === null ? (
                  <td key={c.date} className="text-faint" aria-label={`${r.name} ${label(c.date)} 記録なし`}>
                    ·
                  </td>
                ) : (
                  <td key={c.date}>
                    <button
                      type="button"
                      aria-label={`${r.name} ${label(c.date)} の種目`}
                      aria-pressed={selected?.row === row && selected.col === col}
                      onClick={() => setSelected({ row, col })}
                      className={cn(
                        'mx-auto grid size-9 place-content-center rounded-full text-[15px]',
                        selected?.row === row && selected.col === col
                          ? 'bg-amber/25 text-amber'
                          : 'text-amber',
                      )}
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

      {selected && cell?.exercises && (
        <div className="mt-3 border-t border-line-soft pt-3">
          <p className="text-[13px] text-muted">
            {rows[selected.row]?.name} ・ {label(cell.date)}
          </p>
          <p className="mt-1 text-[15px]">{cell.exercises.map(exerciseLabel).join('・')}</p>
        </div>
      )}
    </Card>
  );
}

function CreateGroup({
  busy,
  note,
  onCreate,
  onCancel,
}: {
  busy: boolean;
  note: string;
  onCreate: (name: string, displayName: string) => Promise<void>;
  onCancel?: () => void;
}) {
  const [name, setName] = useState('');
  const [displayName, setDisplayName] = useState('');
  const problem = nameProblem(name) || nameProblem(displayName);

  return (
    <Card title="グループを作る">
      <div className="grid gap-3">
        <Note>仲間がその週にやった日と種目が見えるようになります。重量は見えません。</Note>
        <LabeledInput label="グループの名前" value={name} onChange={(e) => setName(e.target.value)} />
        <LabeledInput
          label="このグループでの自分の名前"
          value={displayName}
          onChange={(e) => setDisplayName(e.target.value)}
        />
        {note && <Note>{note}</Note>}
        <Button disabled={busy || problem !== ''} onClick={() => void onCreate(name, displayName)}>
          作る
        </Button>
        {onCancel && (
          <Button variant="ghost" size="md" onClick={onCancel}>
            やめる
          </Button>
        )}
      </div>
    </Card>
  );
}

type Confirm = 'rotate' | 'leave' | null;

function GroupActions({
  inviteToken,
  busy,
  onRotate,
  onLeave,
  onCreateAnother,
}: {
  inviteToken: string;
  busy: boolean;
  onRotate: () => Promise<void>;
  onLeave: () => Promise<void>;
  onCreateAnother: () => void;
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
    <Card title="招待とメンバー">
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

        <Button variant="ghost" size="md" disabled={busy} onClick={onCreateAnother}>
          別のグループを作る
        </Button>
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
