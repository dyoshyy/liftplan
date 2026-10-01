// createGate は「同時に1つだけ」走らせる門を作る。
//
// 実行中に来た呼び出しは、待たせずに捨てる。順番に走らせ直す（キューに積む）
// と、連打した分だけ同じ操作がもう一度走って、二重に記録される。
//
// 記録は待ち行列への書き込み（IndexedDB）を待つ。その間もシートは開いたままで
// 「記録する」が押せる。スマホは書き込みが遅いので、人の連打でも2回目が
// 間に合い、別のIDで同じセットが2件入っていた。
//
// **実行中の印は、最初の await より前に立てる。**React の状態にしないのも
// 同じ理由で、状態の更新は次の描画まで見えないので、同じ tick の2回目が
// 素通りする。
export function createGate(): (work: () => Promise<void>) => Promise<void> {
  let busy = false;
  return async (work) => {
    if (busy) return;
    busy = true;
    try {
      await work();
    } finally {
      busy = false;
    }
  };
}
