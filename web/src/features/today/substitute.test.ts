import { describe, expect, it } from 'vitest';
import type { PlannedSet } from '../../api/types';
import {
  accessoryCards,
  canMarkBlocked,
  decodeSwaps,
  encodeSwaps,
  rejectedOf,
  substituteErrorMessage,
  substituteQuery,
  swapsFor,
  withResult,
  type SwapState,
} from './substitute';

const set = (id: string): PlannedSet => ({
  exercise_id: id,
  weight_kg: null,
  sets: 3,
  target_rir: 2,
  target_reps: 10,
});
const empty = (date: string): SwapState => ({ date, swaps: new Map() });

describe('withResult / rejectedOf', () => {
  it('代わりの代わりは、元の replace のまま出した代わりを全部 rejected に積む', () => {
    let s = withResult(empty('2026-10-11'), '2026-10-11', 'leg_curl', set('nordic'));
    expect(rejectedOf(s.swaps, 'leg_curl')).toEqual(['nordic']);
    s = withResult(s, '2026-10-11', 'leg_curl', set('ghr'));
    expect(rejectedOf(s.swaps, 'leg_curl')).toEqual(['nordic', 'ghr']);
    expect(s.swaps.get('leg_curl')?.current?.exercise_id).toBe('ghr');
  });

  it('代わりなしでも、それまでに出した代わりは残す', () => {
    let s = withResult(empty('2026-10-11'), '2026-10-11', 'leg_curl', set('nordic'));
    s = withResult(s, '2026-10-11', 'leg_curl', null);
    expect(s.swaps.get('leg_curl')).toEqual({ replace: 'leg_curl', shown: ['nordic'], current: null });
  });

  it('まだ押していない種目の rejected は空', () => {
    expect(rejectedOf(new Map(), 'leg_curl')).toEqual([]);
  });
});

describe('swapsFor', () => {
  it('日付が変わったら差し替えは持ち越さない', () => {
    const s = withResult(empty('2026-10-11'), '2026-10-11', 'leg_curl', set('nordic'));
    expect(swapsFor(s, '2026-10-11').size).toBe(1);
    expect(swapsFor(s, '2026-10-12').size).toBe(0);
  });

  it('別の日付の状態に結果を書くと、前の日の差し替えは捨てる', () => {
    const s = withResult(empty('2026-10-11'), '2026-10-11', 'leg_curl', set('nordic'));
    const next = withResult(s, '2026-10-12', 'curl', set('hammer'));
    expect([...next.swaps.keys()]).toEqual(['curl']);
  });
});

describe('accessoryCards', () => {
  it('差し替えた補助はその場所に代わりのカードが出る', () => {
    const s = withResult(empty('d'), 'd', 'leg_curl', set('nordic'));
    const cards = accessoryCards([set('curl'), set('leg_curl'), set('incline')], s.swaps);
    expect(cards.map((c) => c.exercise_id)).toEqual(['curl', 'nordic', 'incline']);
    expect(cards[1]?.substitute_for).toBe('leg_curl');
    expect(cards[0]?.substitute_for).toBeUndefined();
  });

  it('代わりなしなら元のカードに no_substitute が付く', () => {
    const s = withResult(empty('d'), 'd', 'leg_curl', null);
    const cards = accessoryCards([set('leg_curl')], s.swaps);
    expect(cards[0]).toMatchObject({ exercise_id: 'leg_curl', no_substitute: true });
  });
});

describe('canMarkBlocked', () => {
  it('記録の無い補助・代わりには出す', () => {
    expect(canMarkBlocked(set('leg_curl'), 0)).toBe(true);
    expect(canMarkBlocked({ ...set('nordic'), substitute_for: 'leg_curl' }, 0)).toBe(true);
  });

  it('1セットでも記録したら出さない（代わりが今日すでにやった種目でも同じ）', () => {
    expect(canMarkBlocked(set('leg_curl'), 1)).toBe(false);
    expect(canMarkBlocked({ ...set('nordic'), substitute_for: 'leg_curl' }, 2)).toBe(false);
  });

  it('代わりなしと分かったカードには出さない', () => {
    expect(canMarkBlocked({ ...set('leg_curl'), no_substitute: true }, 0)).toBe(false);
  });
});

describe('substituteQuery', () => {
  it('表示している計画の日付と、繰り返しの rejected で組む', () => {
    expect(substituteQuery('2026-10-11', 'leg_curl', ['nordic', 'ghr'])).toBe(
      '/api/sessions/substitute?date=2026-10-11&replace=leg_curl&rejected=nordic&rejected=ghr',
    );
  });

  it('rejected が無ければ付けない', () => {
    expect(substituteQuery('2026-10-11', 'leg_curl', [])).toBe(
      '/api/sessions/substitute?date=2026-10-11&replace=leg_curl',
    );
  });
});

describe('encodeSwaps / decodeSwaps', () => {
  it('残したものを読み戻すと同じ差し替えになる', () => {
    let s = withResult(empty('2026-10-11'), '2026-10-11', 'leg_curl', set('nordic'));
    s = withResult(s, '2026-10-11', 'curl', null);
    const back = decodeSwaps(JSON.parse(JSON.stringify(encodeSwaps(s))));
    expect(back.date).toBe('2026-10-11');
    expect([...back.swaps.entries()]).toEqual([...s.swaps.entries()]);
  });

  it('壊れた値・形の違う値は空から始める', () => {
    for (const raw of [
      null,
      'x',
      42,
      {},
      { date: 1, swaps: [] },
      { date: 'd', swaps: [{ replace: 1 }] },
      {
        date: 'd',
        swaps: [
          {
            replace: 'a',
            shown: ['b'],
            current: { exercise_id: 'b', weight_kg: null, target_rir: 2, target_reps: 10 },
          },
        ],
      },
    ]) {
      const s = decodeSwaps(raw);
      expect(s.swaps.size).toBe(0);
    }
  });
});

describe('substituteErrorMessage', () => {
  it('応答が返らなかったときだけオフラインと言う', () => {
    expect(substituteErrorMessage(new TypeError('Failed to fetch'))).toBe('オフラインでは出せません');
    expect(substituteErrorMessage(new Error('500'))).toBe('代わりを読み込めませんでした');
  });
});
