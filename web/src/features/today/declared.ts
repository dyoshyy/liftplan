/**
 * toggleDeclared はチェックの入り切りを反映した宣言種目を返す。
 *
 * 昇順に保つのはサーバーが正規化して返すのに合わせるため。並びが違うと
 * 保存のたびに画面の順序が入れ替わる。
 */
export function toggleDeclared(current: readonly string[], id: string): string[] {
  const next = current.includes(id)
    ? current.filter((x) => x !== id)
    : [...current, id];
  return [...next].sort();
}

/**
 * lockedDeclared は外せない種目を返す。
 *
 * 2つある。
 *
 * 1. **重点種目**。宣言から外すとサーバーが 400 を返す。黙って重点を
 *    解除しないのが決めごとなので、画面側で 400 に到達させない。
 * 2. **最後の1つ**。宣言が空のプログラムは存在しない（NewProgram が弾く）。
 *
 * 選べない理由を出し分けるため、ID ではなく理由つきで返す。
 */
export function lockedDeclared(
  declared: readonly string[],
  focus: string | null,
): Map<string, string> {
  const out = new Map<string, string>();
  if (declared.length === 1 && declared[0] !== undefined) {
    out.set(declared[0], '最後の1つは外せません');
  }
  if (focus !== null && declared.includes(focus)) {
    out.set(focus, '重点種目です。先に重点種目を外してください');
  }
  return out;
}
