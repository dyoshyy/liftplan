// 設定の判断。副作用は持たない。
//
// 送る前に決まることをここに集める。以前は ProgramSettings.tsx の中にあり、
// DOM を立てないと検査できなかった。

/**
 * asText は週目標を入力欄の文字列にする。
 *
 * 数値のまま持つと、入力中の「1.」や空欄が NaN になって値が飛ぶ。
 * 文字列で持ち、保存のときだけ数値にする。
 */
export const asText = (target: Record<string, number>): Record<string, string> =>
  Object.fromEntries(Object.entries(target).map(([k, v]) => [k, String(v)]));

export type ParseResult =
  | { ok: true; value: Record<string, number> }
  | { ok: false; region: string };

/** parseTarget は入力欄の文字列を週目標に戻す。
 *
 *  どの区分が読めなかったかまで返す。「値が数字ではありません」だけでは、
 *  21区分のどれを直せばよいのか分からない。 */
export function parseTarget(target: Record<string, string>): ParseResult {
  const value: Record<string, number> = {};
  for (const [region, text] of Object.entries(target)) {
    const n = Number.parseFloat(text);
    if (!Number.isFinite(n)) return { ok: false, region };
    value[region] = n;
  }
  return { ok: true, value };
}

/** isDirty は種目の選択が変わったかを見る。
 *
 *  並び順の違いは無視する。並びだけで「変わった」にすると、押していないのに
 *  保存ボタンが出続ける。 */
export const isDirty = (draft: readonly string[], saved: readonly string[]): boolean =>
  [...draft].sort().join(',') !== [...saved].sort().join(',');
