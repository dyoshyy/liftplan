/**
 * 「指定なし」を表す選択肢の値。空文字にしてあるのは、種目IDが
 * 空にならない（サーバーが NewExerciseID で弾く）ため。
 */
export const NO_FOCUS = '';

/**
 * focusBody は PUT /api/program/focus のボディを組む。
 *
 * 「指定なし」は **null で送る**。空文字で送るとサーバーは
 * NewExerciseID に通して 400 を返す。ここを取り違えると、指定の解除
 * だけが黙って失敗する（設定する側は通るので気づきにくい）。
 */
export function focusBody(id: string): { focus_exercise: string | null } {
  return { focus_exercise: id === NO_FOCUS ? null : id };
}

/**
 * focusOptions は選択肢を並べる。先頭は必ず「指定なし」。
 *
 * 重点種目は宣言種目の部分集合なので、候補は declared だけ。選択種目を
 * 並べるとサーバーが 400 を返す選択肢が画面に出る。
 */
export function focusOptions(declared: readonly string[]): string[] {
  return [NO_FOCUS, ...declared];
}
