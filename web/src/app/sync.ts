/** SyncState は同期の見え方。ナビの点の色と説明文を決める。 */
export type SyncState = {
  tone: 'ok' | 'pending' | 'trouble';
  /** label は読み上げと長押しのヒント。押すと何が起きるかまで言う。 */
  label: string;
};

// 同期の状態を1つにまとめる。
//
// 順序が意味を持つ。**送れなかった記録が一番強い。**オフラインは電波が
// 戻れば直るが、捨てた記録は本人が入れ直さない限り戻らない。
export function syncState(pending: number, rejected: number, online: boolean): SyncState {
  if (rejected > 0) {
    return { tone: 'trouble', label: `送れなかった記録 ${rejected} 件` };
  }
  if (!online) {
    return {
      tone: 'trouble',
      label: pending > 0 ? `オフライン・未送信 ${pending} 件` : 'オフライン',
    };
  }
  if (pending > 0) {
    return { tone: 'pending', label: `未送信 ${pending} 件。押すと送る` };
  }
  return { tone: 'ok', label: '同期済み。押すと取り直す' };
}
