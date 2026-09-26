import type { Login } from '../../api/types';

const PROVIDER_NAMES: Record<string, string> = { github: 'GitHub', google: 'Google' };

/** providers はログイン方法の名前を並べる。知らないプロバイダは名前のまま出す。 */
const providers = (logins: readonly Login[]): string =>
  logins.map((l) => PROVIDER_NAMES[l.provider] ?? l.provider).join('・');

/** emails は重なりを除いたアドレス。
 *
 *  同じ人の GitHub と Google は同じアドレスで結ばれているので、そのまま
 *  並べると同じものが2回出る。 */
const emails = (logins: readonly Login[]): string[] => [
  ...new Set(logins.flatMap((l) => (l.email ? [l.email] : []))),
];

/**
 * accountSummary は「アカウント」を畳んだときに出すアドレス。
 *
 * 無ければ出さない。見出しに「記録されていません」を出すと、毎回目に入る
 * 場所に毎回同じ言い訳が並ぶ。無いことは開いた中で言う（accountLine）。
 */
export function accountSummary(logins: readonly Login[]): string | undefined {
  const found = emails(logins);
  return found.length > 0 ? found.join('・') : undefined;
}

/**
 * accountLine は「アカウント」を開いたときに出す1行。
 *
 * アドレスを持たないアカウント（0010 より前に作られたもの）では、無いこと
 * をはっきり言う。黙ると、表示が壊れているのか取っていないのかが分からない。
 * アカウントを持たない利用者（開発用のセッション）には何も言わない。
 */
export function accountLine(logins: readonly Login[]): string | null {
  if (logins.length === 0) return null;
  const found = emails(logins);
  if (found.length === 0) return `${providers(logins)} でログインしています。メールアドレスは記録されていません`;
  return `${found.join('・')}（${providers(logins)}）でログインしています`;
}
