import { useState } from 'react';
import { Button } from '../../ui/Button';
import { Card, Note } from '../../ui/Card';
import { LabeledInput } from '../../ui/Field';
import { nameProblem } from './groups';
import type { PendingInvite } from './usePendingInvite';

// JoinInvite は招待リンクを開いたあとに出すカード。どの画面の上にも出す。
//
// リンクを開いた直後はログインの往復を挟むことがあり、戻ってきた先は
// 今日の画面になる。そこで参加を聞かないと、リンクを開いた意味が消える。
export function JoinInvite({ invite }: { invite: PendingInvite }) {
  const [displayName, setDisplayName] = useState('');
  const { view } = invite;

  switch (view.kind) {
    case 'none':
      return null;
    case 'loading':
      return <Card title="招待">招待を確かめています…</Card>;
    case 'ask':
      return (
        <Card title="招待" tone="amber">
          <div className="grid gap-3">
            <p className="text-[15px]">
              「{view.name}」（{view.memberCount}人）に参加しますか？
            </p>
            <Note>参加すると、その週にやった日と種目がお互いに見えます。重量は見えません。</Note>
            <LabeledInput
              label="このグループでの自分の名前"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
            />
            {invite.note && <Note>{invite.note}</Note>}
            <div className="grid grid-cols-2 gap-2">
              <Button variant="quiet" size="md" disabled={invite.busy} onClick={invite.dismiss}>
                参加しない
              </Button>
              <Button
                size="md"
                disabled={invite.busy || nameProblem(displayName) !== ''}
                onClick={() => void invite.join(displayName)}
              >
                参加する
              </Button>
            </div>
          </div>
        </Card>
      );
    case 'already':
      return (
        <Card title="招待">
          <div className="grid gap-3">
            <p className="text-[15px]">「{view.name}」には参加しています。</p>
            <div className="grid grid-cols-2 gap-2">
              <Button variant="quiet" size="md" onClick={invite.dismiss}>
                閉じる
              </Button>
              <Button size="md" onClick={invite.open}>
                見る
              </Button>
            </div>
          </div>
        </Card>
      );
    case 'invalid':
      return (
        <Card title="招待">
          <div className="grid gap-3">
            <p className="text-[15px]">このリンクは使えません。メンバーに新しいリンクをもらってください。</p>
            <Button variant="quiet" size="md" onClick={invite.dismiss}>
              閉じる
            </Button>
          </div>
        </Card>
      );
    case 'unreachable':
      return (
        <Card title="招待">
          <div className="grid gap-3">
            <p className="text-[15px]">{view.message}</p>
            <Button variant="quiet" size="md" onClick={invite.dismiss}>
              招待を捨てる
            </Button>
          </div>
        </Card>
      );
  }
}
