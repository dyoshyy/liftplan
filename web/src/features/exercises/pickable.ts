import type { Exercise } from '../../api/types';

/**
 * pickableExercises は種目を選ぶシートの選択肢。
 *
 * excluded に入っている種目は入れない（今日の画面なら予定に出ている種目、
 * 履歴ならその日に記録がある種目）。選んでも同じ種目が2つ並ぶだけで、
 * 既にあるほうに足せば済む。消した種目と、使わない種目（selected に無いもの）も
 * 入れない。
 * selected が null（読めていない）なら絞らない。一覧が空になって何も選べない
 * より、使わない種目が混ざるほうがよい。
 *
 * 並びはサーバーが返した順のまま。名前の一部で絞れる（前後の空白と
 * 英字の大文字小文字は無視する）。
 */
export function pickableExercises(
  exercises: readonly Exercise[],
  excluded: ReadonlySet<string>,
  query: string,
  selected: readonly string[] | null,
): Exercise[] {
  const q = query.trim().toLowerCase();
  return exercises.filter(
    (e) =>
      !e.deleted &&
      !excluded.has(e.id) &&
      (selected === null || selected.includes(e.id)) &&
      e.name.toLowerCase().includes(q),
  );
}
