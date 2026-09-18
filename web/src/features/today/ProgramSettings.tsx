import { useState } from 'react';
import { getJSON, send } from '../../api/client';
import type { Program } from '../../api/types';
import { focusBody, focusOptions, NO_FOCUS } from './focus';
import { lockedDeclared, lockedSelected, toggleDeclared } from './declared';
import { regionLabel } from '../../domain/regions';
import { Button } from '../../ui/Button';
import { Card, Note } from '../../ui/Card';
import { LabeledInput } from '../../ui/Field';

/**
 * asText は週目標を入力欄の文字列にする。
 *
 * 数値のまま持つと、入力中の「1.」や空欄が NaN になって値が飛ぶ。
 * 文字列で持ち、保存のときだけ数値にする。
 */
function asText(target: Record<string, number>): Record<string, string> {
  return Object.fromEntries(Object.entries(target).map(([k, v]) => [k, String(v)]));
}

type Props = {
  /** 種目IDを表示名にする。 */
  nameOf: (id: string) => string;
  /** 種目マスタ全件のID。使う種目の候補になる。 */
  allExerciseIds: readonly string[];
  /** 変更後にメニューを取り直す。設定はその日の献立を変える。 */
  onChanged: () => Promise<void>;
};

/**
 * 伸ばしたい種目と重点種目を変える。
 *
 * 畳んであるのは、ジムで開く画面の面積を増やさないため（D-120）。
 * プログラムを取りに行くのも開いたときだけで、毎回の読み込みには混ぜない。
 *
 * 2つを1つの部品にしているのは、どちらも同じプログラムを見ているため。
 * 別々に持つと、宣言を変えたあとに重点種目の選択肢が古いままになる。
 *
 * 待ち行列を通さずその場で送る。待ち行列は記録を守るための仕組みで、
 * 設定を混ぜると圏外で押した変更がジムを出たあとに流れ、その日の
 * メニューは変わらないまま「変えたつもり」になる。失敗しても記録は
 * 1件も失わないので、その場で成否を見せるほうが正直。
 */
export function ProgramSettings({ nameOf, allExerciseIds, onChanged }: Props) {
  const [open, setOpen] = useState(false);
  const [program, setProgram] = useState<Program | null>(null);
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);

  // 保存前のチェック状態。宣言はチェックを何個か動かしてから保存する。
  // 1つ動かすたびに送ると、そのたびにメニューが組み替わる。
  const [draft, setDraft] = useState<string[] | null>(null);
  const [pick, setPick] = useState<string[] | null>(null);
  const [target, setTarget] = useState<Record<string, string> | null>(null);

  const expand = async () => {
    setOpen(true);
    if (program) return;
    setNote('');
    try {
      const p = await getJSON<Program>('/api/program');
      setProgram(p);
      setDraft(p.declared_exercises);
      setPick(p.selected_exercises);
      setTarget(asText(p.weekly_target));
    } catch {
      setNote('設定を読めませんでした');
    }
  };

  // put は1フィールドだけの口へ送る。成否をそのまま返す。
  const put = async (path: string, body: unknown): Promise<boolean> => {
    if (!navigator.onLine) {
      setNote('つながらないので変えられません');
      return false;
    }
    setBusy(true);
    setNote('');
    try {
      const res = await send({ path, method: 'PUT', body });
      if (!res.ok) {
        setNote(`変えられませんでした（${res.status}）`);
        return false;
      }
      return true;
    } catch {
      setNote('つながらないので変えられません');
      return false;
    } finally {
      setBusy(false);
    }
  };

  const chooseFocus = async (id: string) => {
    if (!program || busy) return;
    if (!(await put('/api/program/focus', focusBody(id)))) return;
    setProgram({ ...program, focus_exercise: id === NO_FOCUS ? null : id });
    await onChanged();
  };

  const saveDeclared = async () => {
    if (!program || !draft || busy) return;
    if (!(await put('/api/program/declared', { declared_exercises: draft }))) return;
    setProgram({ ...program, declared_exercises: draft });
    await onChanged();
  };

  const saveSelected = async () => {
    if (!program || !pick || busy) return;
    if (!(await put('/api/program/selected', { selected_exercises: pick }))) return;
    setProgram({ ...program, selected_exercises: pick });
    await onChanged();
  };

  const saveTarget = async () => {
    if (!program || !target || busy) return;
    const parsed: Record<string, number> = {};
    for (const [region, text] of Object.entries(target)) {
      const v = Number.parseFloat(text);
      if (!Number.isFinite(v)) {
        setNote(`${regionLabel(region)} の値が数字ではありません`);
        return;
      }
      parsed[region] = v;
    }
    if (!(await put('/api/program/target', { weekly_target: parsed }))) return;
    setProgram({ ...program, weekly_target: parsed });
    await onChanged();
  };

  const saveFrequency = async (n: number) => {
    if (!program || busy) return;
    if (!(await put('/api/program/frequency', { per_week: n }))) return;
    // 週目標も置き直るので、画面の手持ちは捨てて取り直す。
    setProgram(null);
    setDraft(null);
    setPick(null);
    setTarget(null);
    await onChanged();
    await expand();
  };

  if (!open) {
    return (
      <Button variant="quiet" onClick={() => void expand()}>
        メニューの設定を変える
      </Button>
    );
  }

  const locked = program ? lockedDeclared(draft ?? [], program.focus_exercise) : new Map();
  const dirty =
    program && draft
      ? draft.join(',') !== [...program.declared_exercises].sort().join(',')
      : false;
  const targetDirty =
    program && target
      ? JSON.stringify(target) !== JSON.stringify(asText(program.weekly_target))
      : false;
  const pickDirty =
    program && pick
      ? pick.join(',') !== [...program.selected_exercises].sort().join(',')
      : false;

  return (
    <Card title="週に通う回数">
      <Note className="mb-3">
        変えると週目標も回数に合わせて置き直されます。1週間に積めるセット数は
        通う回数に比例するので、片方だけ動かすと目標が実態を説明しなくなります。
      </Note>

      {program && (
        <div className="grid grid-cols-4 gap-2">
          {[1, 2, 3, 4].map((n) => (
            <Button
              key={n}
              variant={program.per_week === n ? 'primary' : 'quiet'}
              disabled={busy}
              onClick={() => void saveFrequency(n)}
            >
              週{n}
            </Button>
          ))}
        </div>
      )}

      <p className="mb-3 mt-5 text-xs uppercase tracking-[0.12em] text-faint">伸ばしたい種目</p>
      <Note className="mb-3">
        ここに入れた種目が、毎回1つずつ順に「軸」として出ます。最後にやったのが
        最も古いものが選ばれるので、数を増やすほど1種目あたりの頻度は下がります。
      </Note>

      {program && draft && (
        <div className="grid gap-2">
          {program.selected_exercises.map((id) => {
            const on = draft.includes(id);
            const why = locked.get(id);
            return (
              <Button
                key={id}
                variant={on ? 'primary' : 'quiet'}
                disabled={busy || (on && why !== undefined)}
                title={why}
                onClick={() => setDraft(toggleDeclared(draft, id))}
              >
                {on ? '✓ ' : ''}
                {nameOf(id)}
                {why && on ? ` — ${why}` : ''}
              </Button>
            );
          })}
        </div>
      )}

      {dirty && (
        <Button
          className="mt-3"
          disabled={busy}
          onClick={() => void saveDeclared()}
        >
          伸ばしたい種目を保存する
        </Button>
      )}

      <p className="mb-3 mt-5 text-xs uppercase tracking-[0.12em] text-faint">重点種目</p>
      <Note className="mb-3">
        選んだ種目の派生（ナローグリップ、テンポなど）が、軸とは別の枠で
        中1日以上あけて出ます。指定しなければバリエーションは出ません。
      </Note>

      {program && (
        <div className="grid gap-2">
          {focusOptions(program.declared_exercises).map((id) => {
            const chosen = (program.focus_exercise ?? NO_FOCUS) === id;
            return (
              <Button
                key={id || 'none'}
                variant={chosen ? 'primary' : 'quiet'}
                disabled={busy}
                onClick={() => void chooseFocus(id)}
              >
                {id === NO_FOCUS ? '指定しない' : nameOf(id)}
              </Button>
            );
          })}
        </div>
      )}

      <p className="mb-3 mt-5 text-xs uppercase tracking-[0.12em] text-faint">使う種目</p>
      <Note className="mb-3">
        ここに入れた種目だけが補助レーンの候補になります。伸ばしたい種目は
        外せません（先にそちらから外してください）。
      </Note>

      {program && pick && (
        <div className="grid gap-2">
          {allExerciseIds.map((id) => {
            const on = pick.includes(id);
            const why = lockedSelected(pick, program.declared_exercises).get(id);
            return (
              <Button
                key={id}
                variant={on ? 'primary' : 'quiet'}
                disabled={busy || (on && why !== undefined)}
                title={why}
                onClick={() => setPick(toggleDeclared(pick, id))}
              >
                {on ? '✓ ' : ''}
                {nameOf(id)}
                {why && on ? ` — ${why}` : ''}
              </Button>
            );
          })}
        </div>
      )}

      {pickDirty && (
        <Button
          className="mt-3"
          disabled={busy}
          onClick={() => void saveSelected()}
        >
          使う種目を保存する
        </Button>
      )}

      <p className="mb-3 mt-5 text-xs uppercase tracking-[0.12em] text-faint">週の目標セット数</p>
      <Note className="mb-3">
        区分ごとの1週間の目安です。通う回数を変えると、ここも回数に合わせて
        置き直ります。届かない目標を置くと毎週すべてが赤字になるだけなので、
        不満が出た区分だけ動かすのが楽です。
      </Note>

      {target && (
        <div className="grid gap-2">
          {Object.keys(target)
            .sort()
            .map((region) => (
              <LabeledInput
                key={region}
                label={regionLabel(region)}
                type="number"
                inputMode="decimal"
                step="0.5"
                min="0"
                value={target[region]}
                disabled={busy}
                onChange={(e) => setTarget({ ...target, [region]: e.target.value })}
              />
            ))}
        </div>
      )}

      {targetDirty && (
        <Button
          className="mt-3"
          disabled={busy}
          onClick={() => void saveTarget()}
        >
          週の目標を保存する
        </Button>
      )}

      {note && <p className="mt-2.5 text-[13px] text-red">{note}</p>}
      <Button variant="quiet" className="mt-3" onClick={() => setOpen(false)}>
        閉じる
      </Button>
    </Card>
  );
}
