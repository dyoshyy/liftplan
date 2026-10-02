// createClaims は「同じ操作を1回だけ通す」ためのラッチを作る。React も DOM も知らない。
//
// 記録は、休憩を始める・自己ベストの演出を出す、といった冪等でない副作用を
// 持つ。操作のID（draftId）を固定すれば、保存そのものは2回通しても
// サーバーと手元の記録が吸収するが、副作用は吸収されない。押した瞬間に
// 1回だけ通すために使う。
//
// **印は同期で立てる**（claim は同じ tick の2回目から false を返す）。React の
// 状態にしないのも同じ理由で、状態の更新は次の描画まで見えないので、同じ
// tick の2回目が素通りする。
//
// 以前の門（保存が終わるまで全部を止める）と違い、キーが違えば止めない。
// 保存を待つあいだに続けて記録した「別の操作」を黙って捨てないため。
export type Claims = {
  /** claim は初めてのキーなら true、2回目以降は false を返す。 */
  claim: (key: string) => boolean;
};

export function createClaims(): Claims {
  // キーは短い文字列で、シートを開くたびに1つ増えるだけ。1日の記録の数が
  // 上限なので、掃除はしない。
  const seen = new Set<string>();
  return {
    claim: (key) => {
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    },
  };
}
