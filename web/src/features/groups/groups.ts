import type { GroupSummary, GroupWeek } from '../../api/types';
import { addDays } from '../../domain/date';

/** WEEKDAYS は週の見出し。サーバーの週は月曜始まり（query.weekOf）。 */
export const WEEKDAYS = ['月', '火', '水', '木', '金', '土', '日'] as const;

/** Cell は格子の1マス。exercises が null ならその日はやっていない。 */
export type Cell = { date: string; exercises: string[] | null };

export type Row = { name: string; isMe: boolean; cells: Cell[] };

/**
 * weekRows はグループの週を、メンバー × 月〜日の格子にする。
 *
 * 自分を先頭に置き、残りは参加順のまま。この画面の目的は「自分だけ
 * 空いている」に気づくことなので、自分の行を探させない。
 */
export function weekRows(week: GroupWeek): Row[] {
  const dates = WEEKDAYS.map((_, i) => addDays(week.from, i));
  const rows = week.members.map((m) => {
    const byDate = new Map(m.days.map((d) => [d.date, d.exercises]));
    return {
      name: m.display_name,
      isMe: m.is_me,
      cells: dates.map((date) => ({ date, exercises: byDate.get(date) ?? null })),
    };
  });
  return [...rows.filter((r) => r.isMe), ...rows.filter((r) => !r.isMe)];
}

/** exerciseLabel は種目名を出す形にする。名前を引けなかった種目も行を消さない。 */
export function exerciseLabel(name: string): string {
  return name === '' ? '名前のない種目' : name;
}

/**
 * chooseGroup は開くグループを決める。前に見ていたものがまだ一覧にあれば
 * それ、無ければ一覧の先頭。一覧が空なら null。
 *
 * 抜けたグループを覚えたままにすると、そのグループを開きに行って 404 に
 * なる。一覧に無いものは覚えていないのと同じに扱う。
 */
export function chooseGroup(groups: readonly GroupSummary[], remembered: string | null): string | null {
  if (remembered !== null && groups.some((g) => g.id === remembered)) return remembered;
  return groups[0]?.id ?? null;
}

/**
 * inviteUrl は招待リンクを作る。画面のオリジンに ?invite= を付けるだけ。
 *
 * 画面にはルーターが無いので、クエリ1つで受ける（設計書「招待リンクの流れ」）。
 */
export function inviteUrl(origin: string, token: string): string {
  return `${origin}/?invite=${encodeURIComponent(token)}`;
}

const NAME_MAX = 20;

/**
 * nameProblem は名前の入力に出す一言。問題なければ空文字。
 *
 * サーバー（group.normalizeName）と同じ規則。送ってから 400 で知るより、
 * 打っている間に分かるほうがよい。数えるのは文字数（コードポイント）。
 */
export function nameProblem(value: string): string {
  const v = value.trim();
  if (v === '') return '入力してください';
  if ([...v].length > NAME_MAX) return `${NAME_MAX}文字までです`;
  return '';
}

/**
 * groupErrorMessage は読めなかったときの一言。
 *
 * fetch が投げる TypeError だけをオフラインと呼ぶ（forecastErrorMessage と同じ）。
 * グループの画面は送信待ちの列に乗せないので、つながるまで待ってもらう。
 */
export function groupErrorMessage(e: unknown): string {
  return e instanceof TypeError ? 'つながったら見られます' : '読み込めませんでした';
}

/** describeFailure は送って断られたときの一言。サーバーの理由があればそれを出す。 */
export function describeFailure(status: number, body: { error?: string } | null): string {
  return body?.error ? body.error : `できませんでした（${status}）`;
}
