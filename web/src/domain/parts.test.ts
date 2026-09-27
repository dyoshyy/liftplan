import { describe, expect, it } from 'vitest';
import { groupByPart, partOf, PART_ORDER, regionsByPart, representativeRegion } from './parts';

describe('representativeRegion', () => {
  it('寄与度が一番大きい区分を返す', () => {
    expect(representativeRegion({ CHEST_MID: 1, TRICEPS_LONG: 0.4 })).toBe('CHEST_MID');
  });

  // 同点をどう解いても表示上の違いしかないが、**毎回同じ答えを返すこと**は要る。
  // 順序が揺れると、開き直すたびに種目が別の部位へ移る。
  it('同点は区分名の昇順で決める（毎回同じ答えになる）', () => {
    const a = representativeRegion({ QUAD: 1, GLUTE: 1 });
    const b = representativeRegion({ GLUTE: 1, QUAD: 1 });
    expect(a).toBe(b);
    expect(a).toBe('GLUTE');
  });

  it('分布が空なら空文字（落とさない）', () => {
    expect(representativeRegion({})).toBe('');
  });
});

describe('partOf', () => {
  it.each([
    { region: 'CHEST_UPPER', part: '胸' },
    { region: 'LAT', part: '背中' },
    { region: 'ERECTOR', part: '背中' },
    { region: 'SIDE_DELT', part: '肩' },
    { region: 'TRICEPS_LONG', part: '腕' },
    { region: 'FOREARM', part: '腕' },
    { region: 'QUAD', part: '脚' },
    { region: 'CALF', part: '脚' },
    { region: 'ABS', part: '体幹' },
  ])('$region → $part', ({ region, part }) => {
    expect(partOf(region)).toBe(part);
  });

  // サーバーが区分を足しても画面を落とさない。
  it('知らない区分は「その他」に落とす', () => {
    expect(partOf('NECK')).toBe('その他');
    expect(partOf('')).toBe('その他');
  });
});

describe('groupByPart', () => {
  const items: { id: string; stimulus: Record<string, number> }[] = [
    { id: 'bench', stimulus: { CHEST_MID: 1, TRICEPS_LONG: 0.4 } },
    { id: 'squat', stimulus: { QUAD: 1, GLUTE: 0.6 } },
    { id: 'curl', stimulus: { BICEPS: 1 } },
    { id: 'incline', stimulus: { CHEST_UPPER: 1 } },
  ];

  it('部位ごとにまとめる', () => {
    const got = groupByPart(items);
    expect(got.map((g) => g.part)).toEqual(['胸', '腕', '脚']);
    expect(got[0]?.items.map((i) => i.id)).toEqual(['bench', 'incline']);
  });

  // 上から順に見ていくので、並びが毎回変わると探せない。
  it('部位の並びは固定（体の上から下）', () => {
    expect(PART_ORDER).toEqual(['胸', '背中', '肩', '腕', '脚', '体幹', 'その他']);
  });

  it('中身が空の部位は出さない', () => {
    expect(groupByPart([{ id: 'curl', stimulus: { BICEPS: 1 } }]).map((g) => g.part)).toEqual(['腕']);
  });

  it('渡した順序を部位の中で保つ', () => {
    const got = groupByPart([...items].reverse());
    expect(got[0]?.items.map((i) => i.id)).toEqual(['incline', 'bench']);
  });
});

describe('regionsByPart', () => {
  // 自分の種目を足す画面で、部位を選ぶチップの並び。21区分を全部、
  // 部位の順（上から下）に出す。1つでも欠けると、その部位に効く種目を足せない。
  it('21区分を漏れなく、部位の順に並べる', () => {
    const groups = regionsByPart();
    expect(groups.map((g) => g.part)).toEqual(['胸', '背中', '肩', '腕', '脚', '体幹']);
    const all = groups.flatMap((g) => g.regions);
    expect(all).toHaveLength(21);
    expect(new Set(all).size).toBe(21);
    for (const g of groups) for (const r of g.regions) expect(partOf(r)).toBe(g.part);
  });
});
