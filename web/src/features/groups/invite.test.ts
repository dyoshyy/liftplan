import { describe, expect, it } from 'vitest';
import { keepsPending, planInviteIntake, takeInvite, viewOfPreview } from './invite';

describe('takeInvite', () => {
  it('invite を取り出し、残りのクエリは残す', () => {
    expect(takeInvite('?invite=abc&sim=1')).toEqual({ token: 'abc', rest: '?sim=1' });
  });

  it('invite だけならクエリは空になる', () => {
    expect(takeInvite('?invite=abc')).toEqual({ token: 'abc', rest: '' });
  });

  // inviteUrl が符号化したトークンを、元の値に戻して持つ。
  it('符号化されたトークンを元に戻す', () => {
    expect(takeInvite('?invite=a%2Bb%2Fc').token).toBe('a+b/c');
  });

  it.each(['', '?sim=1', '?invite=', '?invite=%20'])('「%s」には招待が無い', (search) => {
    expect(takeInvite(search)).toEqual({ token: null, rest: search });
  });
});

describe('planInviteIntake', () => {
  // 逆順だと、消した直後に閉じられたときトークンがどこにも残らない。
  it('覚えてから URL から消す', () => {
    expect(planInviteIntake('?invite=abc')).toEqual([
      { kind: 'remember', token: 'abc' },
      { kind: 'clearQuery', rest: '' },
    ]);
  });

  it('招待が無ければ何もしない（URL も触らない）', () => {
    expect(planInviteIntake('?sim=1')).toEqual([]);
  });
});

describe('viewOfPreview', () => {
  const preview = { group_id: 'g', name: 'ジム仲間', member_count: 3, already_member: false };

  it('入っていなければ参加するかを聞く', () => {
    expect(viewOfPreview({ kind: 'ok', preview })).toEqual({ kind: 'ask', name: 'ジム仲間', memberCount: 3 });
  });

  it('既に入っていれば参加の操作は出さない', () => {
    expect(viewOfPreview({ kind: 'ok', preview: { ...preview, already_member: true } })).toEqual({
      kind: 'already',
      name: 'ジム仲間',
      groupId: 'g',
    });
  });

  it('404 は使えないリンク', () => {
    expect(viewOfPreview({ kind: 'status', status: 404 })).toEqual({ kind: 'invalid' });
  });

  // 503 で「使えないリンク」と言うと、本人は捨ててしまう。実際には後で開けば入れる。
  it('404 以外の失敗は確かめられなかったに留める', () => {
    expect(viewOfPreview({ kind: 'status', status: 503 }).kind).toBe('unreachable');
    expect(viewOfPreview({ kind: 'network' }).kind).toBe('unreachable');
  });
});

describe('keepsPending', () => {
  it.each([
    { kind: 'loading', keep: true },
    { kind: 'ask', keep: true },
    { kind: 'unreachable', keep: true },
    { kind: 'invalid', keep: false },
    { kind: 'already', keep: false },
    { kind: 'none', keep: false },
  ] as const)('$kind のあいだ持ち続けるか: $keep', ({ kind, keep }) => {
    const view =
      kind === 'ask'
        ? { kind, name: 'x', memberCount: 1 }
        : kind === 'already'
          ? { kind, name: 'x', groupId: 'g' }
          : kind === 'unreachable'
            ? { kind, message: 'x' }
            : { kind };
    expect(keepsPending(view)).toBe(keep);
  });
});
