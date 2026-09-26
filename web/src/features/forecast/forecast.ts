import type { ForecastSession } from '../../api/types';

/**
 * sessionHeading は回の見出し。0→今日、1→次の回、2以上→n回後。
 * 分割があれば「・ 日の名前」を添える（設計書の画面モック）。
 *
 * 日付を出さない決定（設計書「先の回の呼び方」）の帰結として、
 * 見出しは出席回数だけで表す。
 */
export function sessionHeading(index: number, split: ForecastSession['split']): string {
  const base = index === 0 ? '今日' : index === 1 ? '次の回' : `${index}回後`;
  return split ? `${base} ・ ${split}` : base;
}
