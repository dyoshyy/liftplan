// 筋区分を、画面でまとめるための粗い「部位」に畳む。
//
// **これは表示の都合であって、ドメインの概念ではない。**サーバーは21の筋区分を
// 持ち、粗い部位を意図的に持っていない（`taxonomy.go`：「ベンチが埋めない
// 大胸筋上部を補助で埋める」を表現するには、胸を上部・中部・下部に割る必要が
// あるため）。週ボリュームの管理はその粒度のままでよい。
//
// 一方、38種目を選ぶ画面で21個の見出しを並べると、1グループが平均2種目未満に
// なって一覧として読めない。畳むのは画面の都合なので、ここに置く。
// 日本語のラベルを regions.ts に置いているのと同じ理由。

/** PART_ORDER は見出しの並び。体の上から下へ。
 *
 *  固定するのは、上から順に探すため。開くたびに並びが変わると見つけられない。 */
export const PART_ORDER = ['胸', '背中', '肩', '腕', '脚', '体幹', 'その他'] as const;

export type Part = (typeof PART_ORDER)[number];

const PARTS: Record<string, Part> = {
  CHEST_UPPER: '胸',
  CHEST_MID: '胸',
  CHEST_LOWER: '胸',

  LAT: '背中',
  TRAP_MID: '背中',
  TRAP_UPPER: '背中',
  ERECTOR: '背中',

  FRONT_DELT: '肩',
  SIDE_DELT: '肩',
  REAR_DELT: '肩',

  TRICEPS_LONG: '腕',
  TRICEPS_LATERAL: '腕',
  BICEPS: '腕',
  FOREARM: '腕',

  QUAD: '脚',
  HAMSTRING: '脚',
  GLUTE: '脚',
  ADDUCTOR: '脚',
  CALF: '脚',

  ABS: '体幹',
  OBLIQUE: '体幹',
};

/** partOf は筋区分を部位に畳む。
 *
 *  知らない区分は「その他」に落とす。サーバーが区分を足したときに画面が
 *  落ちるより、まとめ損ねて出るほうがよい。 */
export const partOf = (region: string): Part => PARTS[region] ?? 'その他';

/** representativeRegion は、その種目を代表する筋区分を選ぶ。
 *
 *  寄与度が一番大きいもの。**同点は区分名の昇順で決める。**どう解いても
 *  表示上の違いしかないが、毎回同じ答えを返すことは要る。順序が揺れると、
 *  開き直すたびに種目が別の部位へ移る。
 *
 *  これはドメインの「支配区分」ではない。あちらは「その種目がどの日に出るか」
 *  を決めるためのもので、分割法と一緒に入ると決まっている。 */
export function representativeRegion(stimulus: Record<string, number>): string {
  let best = '';
  let top = -Infinity;

  for (const region of Object.keys(stimulus).sort()) {
    const value = stimulus[region] ?? 0;
    if (value > top) {
      top = value;
      best = region;
    }
  }
  return best;
}

export type HasStimulus = { stimulus: Record<string, number> };

export type PartGroup<T> = { part: Part; items: T[] };

/** groupByPart は種目を部位ごとにまとめる。
 *
 *  中身が空の部位は出さない。見出しだけが並ぶと、選べるものを探すのに
 *  かえって目が滑る。渡した順序は部位の中で保つ（呼び手が並べた意図を壊さない）。 */
export function groupByPart<T extends HasStimulus>(items: readonly T[]): PartGroup<T>[] {
  const buckets = new Map<Part, T[]>();

  for (const item of items) {
    const part = partOf(representativeRegion(item.stimulus));
    const bucket = buckets.get(part);
    if (bucket) bucket.push(item);
    else buckets.set(part, [item]);
  }

  return PART_ORDER.filter((p) => buckets.has(p)).map((part) => ({
    part,
    items: buckets.get(part) ?? [],
  }));
}

/** regionsByPart は21の筋区分を部位ごとに並べて返す。
 *
 *  自分の種目を足すときに、効く部位を選ばせる並び。部位の中は PARTS に
 *  書いた順（体の上から下）で、開くたびに並びが変わらないようにする。 */
export function regionsByPart(): { part: Part; regions: string[] }[] {
  const buckets = new Map<Part, string[]>();
  for (const [region, part] of Object.entries(PARTS)) {
    const bucket = buckets.get(part);
    if (bucket) bucket.push(region);
    else buckets.set(part, [region]);
  }
  return PART_ORDER.filter((p) => buckets.has(p)).map((part) => ({ part, regions: buckets.get(part) ?? [] }));
}
