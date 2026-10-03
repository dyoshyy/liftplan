// シミュレーション結果の要約。画面の先頭に文字で出す。
//
// Claude が画面の文字（get_page_text）を読んで、変更が効いているかを
// まずここだけで判断できるようにする。グラフは人が形を見るためのもので、
// 数字の確かめはこちら。

import { PART_ORDER, partOf } from '../domain/parts';
import { regionLabel, regionOrder } from '../domain/regions';
import { shortDate, type WeightSeries } from './chart';
import { outOfRange, type DevSettings, type DevWeek } from './simulate';

const kg = (v: number) => `${Math.round(v * 10) / 10}kg`;

/** liftLine は宣言種目1つの推移を1行にする。
 *
 *  初回（何から始めたか）、最後の軸（どこまで来たか）とその日の実力に
 *  対する比（処方が実力を追えているか）、処方の幅、実力の推移。 */
export function liftLine(s: WeightSeries): string {
  if (s.points.length === 0) return `${s.name}: 一度も出ていない`;

  const first = s.points[0]!;
  const last = s.points[s.points.length - 1]!;
  const parts = [`${s.name}: ${s.points.length}回`];

  parts.push(`初回 ${kg(first.kg)}×${first.reps} RIR${first.rir}${first.chosen ? '（本人が選んだ）' : ''}`);

  const mains = s.points.filter((p) => p.lane === 'main' && p.prescribedKg !== null);
  const lastMain = mains[mains.length - 1];
  parts.push(
    lastMain
      ? `最後の軸 ${shortDate(lastMain.date)} ${kg(lastMain.kg)}（実力 ${kg(lastMain.athlete1rm)} の ${Math.round(
          (lastMain.kg / lastMain.athlete1rm) * 100,
        )}%）`
      : '軸に立っていない',
  );

  const prescribed = s.points.flatMap((p) => (p.prescribedKg === null ? [] : [p.prescribedKg]));
  if (prescribed.length > 0) {
    parts.push(`処方 ${Math.min(...prescribed)}〜${kg(Math.max(...prescribed))}`);
  }

  return `${parts.join('。')}。実力 ${Math.round(first.athlete1rm * 10) / 10}→${kg(last.athlete1rm)}`;
}

/** weekLine は1週の充足を1行にする。許容外の区分だけを名前と%で並べる。 */
export function weekLine(w: DevWeek): string {
  // 目標0は「狙わない区分」で、許容外ではない（ヒートマップも空欄）。
  const bad = outOfRange({ ...w, regions: w.regions.filter((r) => r.target > 0) });
  if (bad.length === 0) return `${w.index}週: 全区分が 60〜145% に収まっている`;

  const order = (region: string) => PART_ORDER.indexOf(partOf(region));
  const listed = [...bad]
    .sort((a, b) => order(a.region) - order(b.region))
    .map((r) => `${regionLabel(r.region)} ${Math.round(r.value * 100)}%`);
  return `${w.index}週: 許容外 ${bad.length}区分 — ${listed.join(', ')}`;
}

/** settingsLine は結果を作った前提を1行にする。サーバーが解決した値をそのまま書く。
 *
 *  1RM は宣言種目だけ。全種目を並べると1行に収まらず、読む側が探すことになる
 *  （全種目の値は API の応答の settings にある）。 */
export function settingsLine(s: DevSettings, names: Record<string, string>, splitName: string): string {
  const name = (id: string) => names[id] ?? id;
  const a = s.athlete;
  const orm = s.declared.map((id) => `${name(id)} ${kg(a.one_rep_max_kg?.[id] ?? 0)}`);
  const days = s.weekdays.map((d) => weekdayName(s.start, d)).join('');
  return (
    `前提: ${s.start} から${s.weeks}週・週${s.frequency}回（${days}）・` +
    `1回 ${s.exercises_per_session}種目×${s.sets_per_exercise}セット・分割 ${splitName || 'なし'}・` +
    `重点 ${s.focus ? name(s.focus) : 'なし'}${repsLine(s, name)}。` +
    `模擬ユーザー: 伸び ${a.growth_pct_per_week}%/週・初回は実力の${a.first_session_pct}%・` +
    `体重${a.body_weight_kg}kg。初日の1RM: ${orm.join(', ')}` +
    customLine(s)
  );
}

/** repsLine は設定したレップ数を重点の直後に足す。既定のままなら空。
 *
 *  書式は 種目名 重い日/軽い日。レップ数で重さが動くので、書かないと
 *  結果の数字がどの前提のものか要約だけで分からない。 */
function repsLine(s: DevSettings, name: (id: string) => string): string {
  const entries = Object.entries(s.reps ?? {});
  if (entries.length === 0) return '';
  return `・レップ ${entries.map(([id, r]) => `${name(id)} ${r.heavy}/${r.light}`).join(', ')}`;
}

/** customLine は自分の種目を前提の末尾に足す。無ければ空。
 *
 *  効き方は寄与度の大きい区分から並べる。同点は体の上から下の並びで決着
 *  する（`regionOrder`）。区分名の文字コード順だと LAT が BICEPS の後ろに
 *  来て、寄与が同じでも並びが体の並びと食い違う。 */
function customLine(s: DevSettings): string {
  if (!s.custom || s.custom.length === 0) return '';
  const items = s.custom.map((c) => {
    const entries = Object.entries(c.stimulus).sort(
      ([ra, va], [rb, vb]) => vb - va || regionOrder(ra) - regionOrder(rb),
    );
    const desc = entries.map(([region, v]) => `${regionLabel(region)} ${v.toFixed(1)}`).join('・');
    return `${c.name}（${desc}）`;
  });
  return `。自分の種目: ${items.join(', ')}`;
}

const WEEKDAY = ['日', '月', '火', '水', '木', '金', '土'];

/** weekdayName は開始日から offset 日目の曜日名。曜日は開始日からの日数で
 *  返ってくるので、開始日が月曜でなくても実際の曜日で書く。 */
export function weekdayName(start: string, offset: number): string {
  const [y, m, d] = start.split('-').map(Number) as [number, number, number];
  return WEEKDAY[new Date(Date.UTC(y, m - 1, d + offset)).getUTCDay()]!;
}
