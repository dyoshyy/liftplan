import type { GroupWeek } from '../../api/types';
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

/** LogEntry は1日の中の1人ぶん。 */
export type LogEntry = { name: string; isMe: boolean; exercises: string[] };

/** LogDay は今週の記録の1日ぶん。 */
export type LogDay = { date: string; entries: LogEntry[] };

/**
 * weekLog は今週の記録を日ごとにまとめる。やった日だけを新しい順に並べる。
 *
 * 格子は「やったかどうか」、こちらは「何をやったか」を見る場所。押さなくても
 * 全部読めるようにする（● を1つずつ押して1行ずつ読むのは見づらかった）。
 */
export function weekLog(week: GroupWeek): LogDay[] {
  return weekRows(week)
    .flatMap((r) => r.cells.map((c) => ({ ...c, name: r.name, isMe: r.isMe })))
    .filter((c) => c.exercises !== null)
    .reduce<LogDay[]>((days, c) => {
      const entry = { name: c.name, isMe: c.isMe, exercises: c.exercises ?? [] };
      const day = days.find((d) => d.date === c.date);
      if (day) day.entries.push(entry);
      else days.push({ date: c.date, entries: [entry] });
      return days;
    }, [])
    .sort((a, b) => b.date.localeCompare(a.date));
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
