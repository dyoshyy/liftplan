import { useState } from 'react';
import { clearToken } from '../../storage/local';
import { Button } from '../../ui/Button';
import { Note } from '../../ui/Card';
import { Section } from '../../ui/Section';
import { Stepper } from '../../ui/Stepper';
import type { RestTimer } from '../timer/useRestTimer';
import { ProgramSettings } from './ProgramSettings';
import type { Exercise } from '../../api/types';

type Props = {
  nameOf: (id: string) => string;
  /** exercises は種目マスタ。部位ごとにまとめるのに刺激の分布が要る。 */
  exercises: readonly Exercise[];
  onChanged: () => Promise<void>;
  timer: RestTimer;
  onForget: () => void;
};

const STEP_MIN = 0.25;

// 歯車から入る画面。毎日は触らないものを集める。
//
// 「今日」の中に畳んで置いていたが、ジムで見る画面に毎日触らないものが
// 同居していた。1日1回も開かないものが、セットの合間に見る画面の面積を
// 取っているのは割に合わない。
export function SettingsScreen({ nameOf, exercises, onChanged, timer, onForget }: Props) {
  const [confirming, setConfirming] = useState(false);

  return (
    <>
      <ProgramSettings nameOf={nameOf} exercises={exercises} onChanged={onChanged} />

      <Section title="休憩の長さ" summary={`${Math.round((timer.durationSec / 60) * 100) / 100}分`}>
        <Stepper
          label="セットを記録したあと、ここから数える"
          value={String(Math.round((timer.durationSec / 60) * 100) / 100)}
          onChange={(v) => timer.setDurationSec(Number.parseFloat(v) * 60)}
          step={STEP_MIN}
          decimal
          min={0}
          suffix="分"
        />
        <Note className="mt-3">
          動いている最中に変えても、いま数えているぶんは伸び縮みしません。次に始めたときから効きます。
        </Note>
      </Section>

      <Section title="この端末">
        {/* confirm() は使わない。ページ全体が止まるうえ、記録の途中なら
            入力中の値が消える。取り消せない操作はその場で二段階にする。 */}
        {confirming ? (
          <>
            <Note className="mb-3">
              この端末からトークンを消します。未送信の記録は消えませんが、入れ直すまで送れません。
            </Note>
            <Button
              variant="danger"
              onClick={() => {
                clearToken();
                onForget();
              }}
            >
              消す
            </Button>
            <Button variant="quiet" className="mt-2" onClick={() => setConfirming(false)}>
              やめる
            </Button>
          </>
        ) : (
          <Button variant="danger" onClick={() => setConfirming(true)}>
            トークンを消す
          </Button>
        )}
      </Section>
    </>
  );
}
