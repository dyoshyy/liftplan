import { useState } from 'react';
import type { Exercise } from '../../api/types';
import { groupByPart } from '../../domain/parts';
import { Button } from '../../ui/Button';
import { BackButton } from '../../ui/BackButton';
import { Note } from '../../ui/Card';
import { ExerciseEditor } from './ExerciseEditor';
import {
  aliveExercises,
  deleteBlockedReason,
  draftOf,
  emptyDraft,
  hideBlockedReason,
  stimulusSummary,
  type ExerciseDraft,
} from './exerciseDraft';
import { useExerciseManager } from './useExerciseManager';

type Props = {
  /** exercises は種目マスタ。読み取りは useLiftplan が持つ。 */
  exercises: readonly Exercise[];
  /** 足す・直す・使う種目の入れ替えが成功したら一覧を取り直す。 */
  onChanged: () => Promise<void>;
  onBack: () => void;
};

/**
 * 種目を使うかどうか決め、効き方を直し、足し、消す画面。設定の「種目」の節から入る。
 *
 * 使わない種目は「使う」を外して非表示にする（計画にも「種目を選んで記録」にも
 * 出ない）。要らない種目は消せる（論理削除。記録と履歴は残る）。伸ばしたい
 * 種目・重点種目の選択は設定に残る。3層：判断は exerciseDraft.ts、手順は
 * useExerciseManager、描画はここ。
 */
export function ExerciseManager({ exercises, onChanged, onBack }: Props) {
  const { declared, selected, note, busy, save, toggleUse, pendingDeleteId, requestDelete, cancelDelete } =
    useExerciseManager(onChanged);
  // null = 一覧、'new' = 足す、Exercise = 直す。フォームは足す・直すで共用する。
  const [target, setTarget] = useState<Exercise | 'new' | null>(null);
  const [draft, setDraft] = useState<ExerciseDraft>(emptyDraft);

  const openAdd = () => {
    // 消す前に足すを開いたら、確認待ちのままフォームへ移らせない。
    cancelDelete();
    setDraft(emptyDraft());
    setTarget('new');
  };

  const openEdit = (e: Exercise) => {
    // openAdd と同じ理由。別行の確認待ちを持ち越さない。
    cancelDelete();
    setDraft(draftOf(e));
    setTarget(e);
  };

  const alive = aliveExercises(exercises);

  return (
    <div className="grid gap-3.5">
      <BackButton label="設定" onClick={onBack} />

      <div>
        <h1 className="text-[19px] font-semibold">種目</h1>
        <Note className="mt-1">
          使う種目を選びます。使わない種目は計画にも「種目を選んで記録」にも出ません。
          効き方の調整、一覧に無い器具の追加、削除もここでできます（削除しても記録と履歴は残ります）。
        </Note>
      </div>

      {note && <Note className="text-red">{note}</Note>}

      {target ? (
        <ExerciseEditor
          draft={draft}
          onChange={setDraft}
          onSave={() =>
            // 送信そのもの（POST か PUT かの振り分け・API 呼び出し）は
            // useExerciseManager の save が持つ。ここは結果を受けて画面を
            // 閉じるだけ（orchestration-hooks：描画は手順を持たない）。
            void save(target, draft).then((ok) => {
              if (ok) setTarget(null);
            })
          }
          onCancel={() => setTarget(null)}
          busy={busy}
        />
      ) : (
        <>
          <Button disabled={busy} onClick={openAdd}>
            種目を追加
          </Button>

          {groupByPart(alive).map((group) => (
            <div key={group.part} className="grid gap-2">
              <p className="text-xs tracking-[0.08em] text-faint">{group.part}</p>
              <ul className="grid gap-2">
                {group.items.map((e) => {
                  const on = selected?.includes(e.id) ?? false;
                  const why = on ? hideBlockedReason(declared, e.id) : null;
                  const whyNotDelete = deleteBlockedReason(declared, e.id);
                  const confirmingDelete = pendingDeleteId === e.id;
                  return (
                    <li
                      key={e.id}
                      className={`grid gap-1.5 rounded-xl border border-line bg-surface p-3 ${
                        selected !== null && !on ? 'opacity-60' : ''
                      }`}
                    >
                      <div className="flex min-w-0 items-center justify-between gap-2">
                        {/* flex-1 は flex-basis を 0 にするので、寄与の要約が
                            折り返さない全角文字列でも、それを基準に幅を決めない
                            （auto のままだと中身の自然な幅が基準になり、隣の
                            ボタンごと画面の外へ押し出す）。min-w-0 は自動最小幅
                            （auto）を 0 に上書きして、実際に縮められるようにする。
                            要約は truncate（nowrap）をやめて折り返す：nowrap は
                            自身の内容を1行の幅として持たせてしまい、縮んだ枠の中で
                            見た目上は切れて隠れるだけで、行の外形には影響しない
                            はずが、実機ではここが崩れて行ごと画面の外に出た。 */}
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-sm font-medium">{e.name}</p>
                          <p className="break-words text-xs text-faint">{stimulusSummary(e.stimulus)}</p>
                        </div>
                        <div className="flex shrink-0 gap-2">
                          <Button
                            size="md"
                            variant="quiet"
                            disabled={busy}
                            aria-label={`${e.name}を編集`}
                            onClick={() => openEdit(e)}
                          >
                            編集
                          </Button>
                          <Button
                            size="md"
                            variant={on ? 'selected' : 'quiet'}
                            disabled={busy || selected === null || why !== null}
                            title={why ?? undefined}
                            aria-pressed={on}
                            aria-label={`${e.name}を使う`}
                            onClick={() => void toggleUse(e.id)}
                          >
                            {on ? '✓ 使う' : '使わない'}
                          </Button>
                        </div>
                      </div>
                      {/* title は指の操作では出ない（ホバーが無い）ので、
                          スマホでも読める場所にも同じ理由を出す
                          （ProgramSettings の分割プリセットと同じ扱い）。 */}
                      {why && <Note>{why}</Note>}
                      {/* 削除は行の下に置く。編集・使うと同じ並びに置くと、狭い
                          画面で名前が押し出されるのと、隣のボタンを押すつもりの
                          指が削除に当たる。確認は2段（nextDeleteStep）。 */}
                      <div className="flex items-center justify-end gap-2">
                        {confirmingDelete ? (
                          <>
                            <span className="mr-auto text-xs text-red">
                              本当に削除しますか？記録と履歴は残ります
                            </span>
                            <Button
                              size="md"
                              variant="quiet"
                              disabled={busy}
                              aria-label={`${e.name}の削除をやめる`}
                              onClick={cancelDelete}
                            >
                              やめる
                            </Button>
                            <Button
                              size="md"
                              variant="danger"
                              disabled={busy}
                              aria-label={`${e.name}を本当に削除`}
                              onClick={() => void requestDelete(e.id)}
                            >
                              本当に削除
                            </Button>
                          </>
                        ) : (
                          <Button
                            size="md"
                            variant="quiet"
                            disabled={busy || whyNotDelete !== null}
                            title={whyNotDelete ?? undefined}
                            aria-label={`${e.name}を削除`}
                            onClick={() => void requestDelete(e.id)}
                          >
                            削除
                          </Button>
                        )}
                      </div>
                      {whyNotDelete && !why && <Note>{whyNotDelete}</Note>}
                    </li>
                  );
                })}
              </ul>
            </div>
          ))}
        </>
      )}
    </div>
  );
}
