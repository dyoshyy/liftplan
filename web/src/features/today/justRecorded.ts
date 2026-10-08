import type { RecordedSet } from '../../api/types';

/**
 * justRecorded は、カードを描いたあとに記録されたセットかを返す。
 *
 * 描いた時点で済んでいたセットは弾ませない。画面を開くたびに済んだ枠が
 * 一斉に跳ねると、いま記録したのがどれか分からなくなる。
 * 枠の番号ではなく ID で見る。番号だと、取り消して記録し直した枠が弾まない。
 * 直したセットは同じ ID で入れ直す（planRecord）ので、直しても弾まない。
 */
export const justRecorded = (seenAtMount: ReadonlySet<string>, rec: RecordedSet | undefined): boolean =>
  rec !== undefined && !seenAtMount.has(rec.id);
