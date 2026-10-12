import { useState } from 'react';
import { Button } from '../../ui/Button';
import { Card, Note } from '../../ui/Card';
import { LabeledInput } from '../../ui/Field';
import { ChevronRightIcon } from '../../ui/icons';
import { Loading, SkeletonCard } from '../../ui/Skeleton';
import { nameProblem } from './groups';
import { useGroupList } from './useGroupList';

// GroupList は自分のグループの一覧。下のナビの「グループ」で開く。
//
// グループへの操作（共有・抜ける）は詳細に、どのグループにも属さない操作
// （作る）はここに置く。同じカードに並べると、どれがどのグループへの操作か
// 読み分けることになる。
export function GroupList({ onOpen }: { onOpen: (id: string) => void }) {
  const list = useGroupList();
  const [creating, setCreating] = useState(false);

  if (list.error) return <Note>{list.error}</Note>;
  if (list.groups === null) {
    return (
      <Loading>
        <SkeletonCard rows={3} />
      </Loading>
    );
  }

  const form = (
    <CreateGroup
      busy={list.busy}
      note={list.note}
      onCancel={list.groups.length > 0 ? () => setCreating(false) : undefined}
      onCreate={async (name, displayName) => {
        const id = await list.create(name, displayName);
        if (id !== null) onOpen(id);
      }}
    />
  );
  if (list.groups.length === 0) return form;

  return (
    <div className="grid gap-3.5">
      <h1 className="text-[19px] font-semibold">グループ</h1>
      <div className="grid overflow-hidden rounded-[14px] border border-line bg-surface">
        {list.groups.map((g, i) => (
          <button
            key={g.id}
            type="button"
            onClick={() => onOpen(g.id)}
            className={`flex min-h-14 items-center gap-3 px-4 py-3 text-left ${i > 0 ? 'border-t border-line-soft' : ''}`}
          >
            <span className="min-w-0 flex-1 truncate text-[16px] font-medium">{g.name}</span>
            <span className="num text-[13px] text-muted">{g.member_count}人</span>
            <ChevronRightIcon width={18} height={18} className="text-faint" />
          </button>
        ))}
      </div>
      {creating ? (
        form
      ) : (
        <Button variant="quiet" size="md" onClick={() => setCreating(true)}>
          グループを作る
        </Button>
      )}
    </div>
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
        <Note>メンバーがその週にやった日と種目が見えるようになります。重量は見えません。</Note>
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
