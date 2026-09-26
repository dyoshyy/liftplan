import type { ForecastSession, PlannedSet } from '../../api/types';

const OFFLINE_MESSAGE = 'オフラインでは見られません';
const COMMUNICATION_ERROR_MESSAGE = '予定を読み込めませんでした';

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

/**
 * forecastRows は1回ぶんの種目を、軸・バリエーション・補助の順に並べる
 * （設計書の画面モックの並び）。空のレーンは詰めて、隙間を作らない。
 */
export function forecastRows(session: ForecastSession): PlannedSet[] {
  return [...session.main, ...session.variation, ...session.accessories];
}

/**
 * isInitiallyOpen はページを開いたときに、その回を開いた状態で見せるか。
 * 今日（回0）だけ開き、先の回はたたんで並べる（設計書の画面モック）。
 */
export function isInitiallyOpen(index: number): boolean {
  return index === 0;
}

/**
 * forecastErrorMessage は取得に失敗したときに出す一言を選ぶ。
 *
 * 「オフラインでは見られません」は**本当に応答が返らなかった**ときだけ
 * 出す（`fetch` が投げる `TypeError`、または `navigator.onLine` が
 * false）。401（`Unauthorized`）や `getJSON` が投げる非2xxの `Error`
 * （409・5xx など）はサーバーとは通信できているので、オフラインだと
 * 言い切ると誤解させる——新設エンドポイントが5xxを返しているだけなのに
 * 「オフラインだから」と読めると、実際に壊れている場所を見誤る。
 * それ以外の失敗は原因を名指ししない中立な一言にする。
 */
export function forecastErrorMessage(e: unknown): string {
  const offline = typeof navigator !== 'undefined' && navigator.onLine === false;
  const noResponseReceived = e instanceof TypeError || offline;
  return noResponseReceived ? OFFLINE_MESSAGE : COMMUNICATION_ERROR_MESSAGE;
}
