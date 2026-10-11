import type { InvitePreview } from '../../api/types';

/**
 * takeInvite は URL のクエリから招待トークンを取り出し、残りのクエリを返す。
 * 無ければ token は null で、クエリはそのまま。
 *
 * 残りを返すのは、ほかのクエリ（開発用の画面の設定など）を消さないため。
 */
export function takeInvite(search: string): { token: string | null; rest: string } {
  const params = new URLSearchParams(search);
  const token = params.get('invite');
  if (token === null || token.trim() === '') return { token: null, rest: search };
  params.delete('invite');
  const rest = params.toString();
  return { token: token.trim(), rest: rest === '' ? '' : `?${rest}` };
}

/** InviteIntakeStep は招待リンクを開いたときに何をするか。実行はしない。 */
export type InviteIntakeStep = { kind: 'remember'; token: string } | { kind: 'clearQuery'; rest: string };

/**
 * planInviteIntake は招待リンクを開いたときに何をどの順ですべきかを決める。
 *
 * 覚えてから URL から消す。逆にすると、その間に閉じたり再読み込みしたり
 * したときにトークンがどこにも残らない。URL に残し続けないのは、ブラウザの
 * 履歴やスクリーンショットにリンクの鍵が残るため（planTokenIntake と同じ考え方）。
 */
export function planInviteIntake(search: string): InviteIntakeStep[] {
  const { token, rest } = takeInvite(search);
  if (token === null) return [];
  return [
    { kind: 'remember', token },
    { kind: 'clearQuery', rest },
  ];
}

/** PreviewOutcome は招待リンクの中身を取りに行った結果。 */
export type PreviewOutcome =
  { kind: 'ok'; preview: InvitePreview } | { kind: 'status'; status: number } | { kind: 'network' };

/** InviteView は招待について画面に何を出すか。 */
export type InviteView =
  | { kind: 'none' }
  | { kind: 'loading' }
  /** ask は参加するかを聞く。表示名を入れてもらう。 */
  | { kind: 'ask'; name: string; memberCount: number }
  /** already は既に入っている。参加の操作は要らない。 */
  | { kind: 'already'; name: string; groupId: string }
  /** invalid はリンクが使えない（作り直された・グループが消えた）。 */
  | { kind: 'invalid' }
  /** unreachable は確かめられなかった。トークンは捨てずに持っておく。 */
  | { kind: 'unreachable'; message: string };

/**
 * viewOfPreview は中身を取りに行った結果から、画面に出すものを決める。
 *
 * 404 だけを「使えない」と言い切る。サーバーはメンバーでない・無い・
 * 作り直されたを同じ 404 にしている（存在を漏らさないため）。それ以外の
 * 失敗はリンクの問題とは限らないので、トークンを捨てずに「確かめられ
 * なかった」に留める。
 */
export function viewOfPreview(outcome: PreviewOutcome): InviteView {
  switch (outcome.kind) {
    case 'ok':
      return outcome.preview.already_member
        ? { kind: 'already', name: outcome.preview.name, groupId: outcome.preview.group_id }
        : { kind: 'ask', name: outcome.preview.name, memberCount: outcome.preview.member_count };
    case 'status':
      return outcome.status === 404
        ? { kind: 'invalid' }
        : { kind: 'unreachable', message: `招待を確かめられませんでした（${outcome.status}）` };
    case 'network':
      return { kind: 'unreachable', message: 'つながったら参加できます' };
  }
}

/**
 * keepsPending は、その表示のあいだ招待トークンを持ち続けるか。
 *
 * 確かめられなかっただけなら持ち続ける（次に開いたときにまた聞く）。
 * 使えないと分かった・既に入っていると分かったら、もう要らない。
 */
export function keepsPending(view: InviteView): boolean {
  return view.kind === 'loading' || view.kind === 'ask' || view.kind === 'unreachable';
}
