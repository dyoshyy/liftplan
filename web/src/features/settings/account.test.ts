import { describe, expect, it } from 'vitest';
import { accountLine, accountSummary } from './account';

const github = (email: string | null) => ({ provider: 'github', email });
const google = (email: string | null) => ({ provider: 'google', email });

describe('accountSummary', () => {
  // 畳んだ見出しに出すのはアドレスだけ。同じ人の GitHub と Google は
  // 同じアドレスで結ばれているので、並べると同じものが2回出る。
  it('同じアドレスは1つにまとめる', () => {
    expect(accountSummary([github('gym@example.com'), google('gym@example.com')])).toBe('gym@example.com');
  });

  it('アドレスが無ければ出さない', () => {
    expect(accountSummary([github(null)])).toBeUndefined();
    expect(accountSummary([])).toBeUndefined();
  });
});

describe('accountLine', () => {
  it('アドレスとログイン方法を出す', () => {
    expect(accountLine([github('gym@example.com'), google('gym@example.com')])).toBe(
      'gym@example.com（GitHub・Google）でログインしています',
    );
  });

  // 0010 より前に作られたアカウントはアドレスを持たない。何も出さないと、
  // 表示が壊れているのか、取っていないのかが分からない。
  it('アドレスが無ければ、無いことを言う', () => {
    expect(accountLine([github(null)])).toBe(
      'GitHub でログインしています。メールアドレスは記録されていません',
    );
  });

  // 開発用のセッションはアカウントを持たない。
  it('アカウントが無ければ何も言わない', () => {
    expect(accountLine([])).toBeNull();
  });

  it('知らないプロバイダは名前をそのまま出す', () => {
    expect(accountLine([{ provider: 'apple', email: 'a@example.com' }])).toBe(
      'a@example.com（apple）でログインしています',
    );
  });
});
