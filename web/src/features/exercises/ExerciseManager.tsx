import { useState } from 'react';
import type { Exercise } from '../../api/types';
import { groupByPart } from '../../domain/parts';
import { Button } from '../../ui/Button';
import { Note } from '../../ui/Card';
import { ExerciseEditor } from './ExerciseEditor';
import {
  aliveExercises,
  deleteBlockedReason,
  draftOf,
  emptyDraft,
  stimulusSummary,
  type ExerciseDraft,
} from './exerciseDraft';
import { useExerciseManager } from './useExerciseManager';

type Props = {
  /** exercises は種目マスタ。読み取りは useLiftplan が持つ。 */
  exercises: readonly Exercise[];
  /** 足す・直す・消すが成功したら一覧を取り直す。 */
  onChanged: () => Promise<void>;
  onBack: () => void;
};

/**
 * 種目を足す・直す・消す画面。設定の「種目」の節から入る。
 *
 * 使う種目・伸ばしたい種目・重点種目の選択は設定に残る。ここは種目マスタ
 * そのものの編集だけを持つ（3層：判断は exerciseDraft.ts、手順は
 * useExerciseManager、描画はここ）。
 */
export function ExerciseManager({ exercises, onChanged, onBack }: Props) {
  const { declared, note, busy, save, pendingDeleteId, requestDelete, cancelDelete } = useExerciseManager(onChanged);
  // null = 一覧、'new' = 足す、Exercise = 直す。フォームは足す・直すで共用する。
  const [target, setTarget] = useState<Exercise | 'new' | null>(null);
  const [draft, setDraft] = useState<ExerciseDraft>(emptyDraft);

  const openAdd = () => {
    setDraft(emptyDraft());
    setTarget('new');
    // 消す前に足すを開いたら、確認待ちのままフォームへ移らせない。
    cancelDelete();
  };

  const openEdit = (e: Exercise) => {
    setDraft(draftOf(e));
    setTarget(e);
    // openAdd と同じ理由。別行の確認待ちを持ち越さない。
    cancelDelete();
  };

  const alive = aliveExercises(exercises);

  return (
    <div className="grid gap-3.5">
      <button type="button" onClick={onBack} className="w-fit text-sm text-muted">
        ← 設定
      </button>

      <div>
        <h1 className="text-[19px] font-semibold">種目</h1>
        <Note className="mt-1">
          自分の器具に合わせて種目を足す・直す・消せます。消しても、これまでの記録と履歴は残ります。
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
            種目を足す
          </Button>

          {groupByPart(alive).map((group) => (
            <div key={group.part} className="grid gap-2">
              <p className="text-xs tracking-[0.08em] text-faint">{group.part}</p>
              <ul className="grid gap-2">
                {group.items.map((e) => {
                  const why = deleteBlockedReason(declared, e.id);
                  const confirmingDelete = pendingDeleteId === e.id;
                  return (
                    <li key={e.id} className="grid gap-1.5 rounded-xl border border-line bg-surface p-3">
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
                            aria-label={`${e.name}を直す`}
                            onClick={() => openEdit(e)}
                          >
                            直す
                          </Button>
                          {confirmingDelete ? (
                            <>
                              <Button
                                size="md"
                                variant="quiet"
                                disabled={busy}
                                aria-label={`${e.name}を消すのをやめる`}
                                onClick={cancelDelete}
                              >
                                やめる
                              </Button>
                              <Button
                                size="md"
                                variant="danger"
                                disabled={busy}
                                aria-label={`${e.name}を本当に消す`}
                                onClick={() => void requestDelete(e.id)}
                              >
                                本当に消す
                              </Button>
                            </>
                          ) : (
                            <Button
                              size="md"
                              variant="danger"
                              disabled={busy || why !== null}
                              title={why ?? undefined}
                              aria-label={`${e.name}を消す`}
                              onClick={() => void requestDelete(e.id)}
                            >
                              消す
                            </Button>
                          )}
                        </div>
                      </div>
                      {/* title は指の操作では出ない（ホバーが無い）ので、
                          スマホでも読める場所にも同じ理由を出す
                          （ProgramSettings の分割プリセットと同じ扱い）。
                          消す確認中は理由を出す必要が無い（why が無いから
                          確認に進めている）。 */}
                      {why && !confirmingDelete && <Note>{why}</Note>}
                      {confirmingDelete && <Note>本当に消しますか？消した種目は直せません（記録は残ります）</Note>}
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
