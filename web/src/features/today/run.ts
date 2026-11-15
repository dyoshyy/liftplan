import type { QueueItem } from '../../outbox/db';

/** RunValues はラン1回の中身。サーバーに送る形のまま持つ。 */
export type RunValues = { distance_km: number; duration_seconds: number };

/** Run は記録済みのラン1回（GET /api/run-logs の1件）。 */
export type Run = RunValues & { id: string; date: string };

// サーバーの範囲（internal/domain/training/runlog）と揃える。ずらすと、
// 押せたのに待ち行列で 400 になり、「送れなかった記録」に落ちる。
const MAX_DISTANCE_METERS = 200_000;
const MAX_DURATION_SECONDS = 24 * 60 * 60;

const WHOLE = /^\d+$/;

/**
 * parseRunInput は距離（km）と時間（分・秒）の入力を読む。読めなければ null。
 *
 * 読めない値では記録ボタンを押せなくするだけで、文で叱らない（体重の行と同じ）。
 * 秒は空なら0秒。距離はサーバーと同じくメートルに丸めてから範囲を見る。
 */
export function parseRunInput(km: string, min: string, sec: string): RunValues | null {
  const distance = Number(km);
  if (!Number.isFinite(distance)) return null;
  const meters = Math.round(distance * 1000);
  if (meters < 1 || meters > MAX_DISTANCE_METERS) return null;

  const m = min.trim();
  const s = sec.trim() === '' ? '0' : sec.trim();
  if (!WHOLE.test(m) || !WHOLE.test(s)) return null;
  const seconds = Number(s);
  if (seconds >= 60) return null;
  const total = Number(m) * 60 + seconds;
  if (total < 1 || total > MAX_DURATION_SECONDS) return null;

  return { distance_km: meters / 1000, duration_seconds: total };
}

/**
 * planRun はランの記録を1件の送信要求にする。
 *
 * id はシートを開いたときに1回だけ決めたもの。押すたびに採番すると、連打した
 * 分だけ別のランになる。同じ id なら、サーバーが2回目を冪等に吸収する。
 */
export function planRun({ id, date, values }: { id: string; date: string; values: RunValues }): QueueItem {
  return { path: '/api/run-logs', body: { id, date, ...values } };
}

/** planUndoRun は記録の取り消しを1件の送信要求にする。 */
export function planUndoRun(id: string): QueueItem {
  return { method: 'DELETE', path: `/api/run-logs/${encodeURIComponent(id)}` };
}

/**
 * visibleRuns は今日の行に出すランを決める。
 *
 * サーバーの一覧に、この画面で記録したもの（まだ待ち行列にあって届いていない
 * かもしれない）を足し、この画面で取り消したものを除く。サーバーの一覧だけを
 * 出すと、記録した直後に消えて見え、もう一度記録して二重になる。
 */
export function visibleRuns(
  server: readonly Run[],
  added: readonly Run[],
  removed: ReadonlySet<string>,
): Run[] {
  const seen = new Set<string>();
  const out: Run[] = [];
  for (const r of [...server, ...added]) {
    if (seen.has(r.id) || removed.has(r.id)) continue;
    seen.add(r.id);
    out.push(r);
  }
  return out;
}

/** formatRun は「5.2km 28:00」の形にする。1時間を超えたら「1:05:07」。 */
export function formatRun({ distance_km, duration_seconds }: RunValues): string {
  const h = Math.floor(duration_seconds / 3600);
  const m = Math.floor((duration_seconds % 3600) / 60);
  const s = duration_seconds % 60;
  const p = (n: number) => String(n).padStart(2, '0');
  const time = h > 0 ? `${h}:${p(m)}:${p(s)}` : `${m}:${p(s)}`;
  return `${distance_km}km ${time}`;
}
