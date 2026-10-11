import { describe, expect, it } from 'vitest';
import type { GroupWeek } from '../../api/types';
import { chooseGroup, exerciseLabel, inviteUrl, nameProblem, weekRows } from './groups';

const week = (members: GroupWeek['members']): GroupWeek => ({
  id: 'g',
  name: 'ジム仲間',
  invite_token: 't',
  from: '2026-10-12',
  to: '2026-10-18',
  members,
});

describe('weekRows', () => {
  it('メンバーごとに月〜日の7マスを作り、やった日にだけ種目を入れる', () => {
    const rows = weekRows(
      week([
        { display_name: 'よし', is_me: true, days: [{ date: '2026-10-14', exercises: ['ベンチプレス'] }] },
      ]),
    );
    expect(rows).toHaveLength(1);
    expect(rows[0]!.cells.map((c) => c.date)).toEqual([
      '2026-10-12',
      '2026-10-13',
      '2026-10-14',
      '2026-10-15',
      '2026-10-16',
      '2026-10-17',
      '2026-10-18',
    ]);
    expect(rows[0]!.cells.map((c) => c.exercises)).toEqual([
      null,
      null,
      ['ベンチプレス'],
      null,
      null,
      null,
      null,
    ]);
  });

  // 目的は「自分だけ空いている」に気づくこと。自分の行を探させない。
  it('自分を先頭に置き、残りは参加順のまま', () => {
    const rows = weekRows(
      week([
        { display_name: 'たろう', is_me: false, days: [] },
        { display_name: 'じろう', is_me: false, days: [] },
        { display_name: 'よし', is_me: true, days: [] },
      ]),
    );
    expect(rows.map((r) => r.name)).toEqual(['よし', 'たろう', 'じろう']);
  });

  // 日曜の記録が落ちると、週末にやった人が「やっていない」に見える。
  it('日曜の記録も最後のマスに入る', () => {
    const rows = weekRows(
      week([
        { display_name: 'よし', is_me: true, days: [{ date: '2026-10-18', exercises: ['スクワット'] }] },
      ]),
    );
    expect(rows[0]!.cells[6]!.exercises).toEqual(['スクワット']);
  });
});

describe('chooseGroup', () => {
  const groups = [
    { id: 'a', name: 'A', member_count: 1, my_name: 'x' },
    { id: 'b', name: 'B', member_count: 2, my_name: 'y' },
  ];

  it('前に見ていたグループがあればそれを開く', () => {
    expect(chooseGroup(groups, 'b')).toBe('b');
  });

  // 抜けたグループを覚えたままだと、開きに行って 404 になる。
  it('前に見ていたグループが一覧に無ければ先頭を開く', () => {
    expect(chooseGroup(groups, 'gone')).toBe('a');
    expect(chooseGroup(groups, null)).toBe('a');
  });

  it('一覧が空なら何も開かない', () => {
    expect(chooseGroup([], 'a')).toBeNull();
  });
});

describe('inviteUrl', () => {
  it('画面のオリジンに invite を付ける。トークンは URL として安全にする', () => {
    expect(inviteUrl('https://liftplan.example', 'a+b/c')).toBe('https://liftplan.example/?invite=a%2Bb%2Fc');
  });
});

describe('nameProblem', () => {
  it.each([
    { value: 'よし', want: '' },
    { value: '  よし  ', want: '' },
    { value: '', want: '入力してください' },
    { value: '　 ', want: '入力してください' },
    // サーバーは文字数で数える。length（UTF-16）で数えると絵文字で食い違う。
    { value: 'あ'.repeat(20), want: '' },
    { value: '😀'.repeat(20), want: '' },
    { value: 'あ'.repeat(21), want: '20文字までです' },
  ])('「$value」→「$want」', ({ value, want }) => {
    expect(nameProblem(value)).toBe(want);
  });
});

describe('exerciseLabel', () => {
  it('名前を引けなかった種目も行を消さずに出す', () => {
    expect(exerciseLabel('')).toBe('名前のない種目');
    expect(exerciseLabel('ベンチプレス')).toBe('ベンチプレス');
  });
});
