import { describe, expect, it } from 'vitest';
import { formatRun, parseRunInput, planRun, planUndoRun, visibleRuns, type Run } from './run';

describe('parseRunInput', () => {
  it('距離と分・秒を、サーバーに送る形に読む', () => {
    expect(parseRunInput('5.2', '28', '30')).toEqual({ distance_km: 5.2, duration_seconds: 28 * 60 + 30 });
  });

  // 秒を入れない人は多い。空を 0 秒と読まないと、28分ちょうどが記録できない。
  it('秒が空なら0秒', () => {
    expect(parseRunInput('5', '28', '')).toEqual({ distance_km: 5, duration_seconds: 1680 });
  });

  // 読めない値では記録ボタンを押せなくする（体重の行と同じ。文で叱らない）。
  // サーバーの範囲（1m〜200km・1秒〜24時間）と揃える。ずらすと、押せたのに
  // 待ち行列で 400 になって「送れなかった記録」に落ちる。
  it.each([
    ['距離が空', '', '28', '0'],
    ['距離が0', '0', '28', '0'],
    ['距離が丸めると0m', '0.0004', '28', '0'],
    ['距離が200kmを超える', '200.001', '28', '0'],
    ['距離が数でない', 'abc', '28', '0'],
    ['時間が0', '5', '0', '0'],
    ['分が空で秒も空', '5', '', ''],
    ['分が小数', '5', '28.5', '0'],
    ['秒が60以上', '5', '28', '60'],
    ['秒が負', '5', '28', '-1'],
    ['24時間を超える', '5', String(24 * 60), '1'],
  ])('%s は読めない', (_name, km, min, sec) => {
    expect(parseRunInput(km, min, sec)).toBeNull();
  });

  it('上限ちょうどは読める', () => {
    expect(parseRunInput('200', String(24 * 60), '0')).toEqual({ distance_km: 200, duration_seconds: 86400 });
  });
});

describe('planRun', () => {
  // ID はシートを開いたときに1回だけ決める（draftId）。押すたびに採番すると、
  // 連打した分だけ別のランになる。同じ ID なら、サーバーが2回目を冪等に吸収する。
  it('開いたときの ID と今日の日付で POST を1件積む', () => {
    expect(
      planRun({ id: 'r1', date: '2026-08-18', values: { distance_km: 5.2, duration_seconds: 1680 } }),
    ).toEqual({
      path: '/api/run-logs',
      body: { id: 'r1', date: '2026-08-18', distance_km: 5.2, duration_seconds: 1680 },
    });
  });
});

describe('planUndoRun', () => {
  // ID はクライアント採番で任意の文字を含みうる。そのまま繋ぐと / や ? で
  // 別のパスを消しに行く。
  it('ID をパスに埋めて DELETE を積む', () => {
    expect(planUndoRun('w-a/b?c')).toEqual({ method: 'DELETE', path: '/api/run-logs/w-a%2Fb%3Fc' });
  });
});

describe('visibleRuns', () => {
  const run = (id: string, km = 5): Run => ({
    id,
    date: '2026-08-18',
    distance_km: km,
    duration_seconds: 1500,
  });

  // 記録した直後は、まだ待ち行列にあってサーバーに届いていない。サーバーの
  // 一覧だけを出すと、記録したのに消えて見え、もう一度記録して二重になる。
  it('サーバーの一覧に、この画面で記録したものを足す', () => {
    expect(visibleRuns([run('a')], [run('b')], new Set()).map((r) => r.id)).toEqual(['a', 'b']);
  });

  // 送れたあとに取り直すと、同じランがサーバーとこの画面の両方にある。
  it('同じ ID は1回だけ出す', () => {
    expect(visibleRuns([run('a')], [run('a')], new Set()).map((r) => r.id)).toEqual(['a']);
  });

  // 取り消した直後も、DELETE はまだ待ち行列にある。サーバーの一覧に残って
  // いても出さない。
  it('この画面で取り消したものは、サーバーにあっても出さない', () => {
    expect(visibleRuns([run('a')], [run('b')], new Set(['a', 'b']))).toEqual([]);
  });
});

describe('formatRun', () => {
  it('距離と分:秒で出す', () => {
    expect(formatRun({ distance_km: 5.2, duration_seconds: 1680 })).toBe('5.2km 28:00');
  });

  it('1時間を超えたら時:分:秒', () => {
    expect(formatRun({ distance_km: 21.1, duration_seconds: 3600 + 5 * 60 + 7 })).toBe('21.1km 1:05:07');
  });
});
