import { describe, expect, it } from 'vitest';
import { Unauthorized } from '../api/client';
import {
  buildQuery,
  curlCommand,
  describeFailure,
  defaultForm,
  parseForm,
  setOneRepMax,
  toggleDay,
  formatPct,
  formatPerformed,
  formatWeight,
  outOfRange,
  rate,
  toggle,
  tone,
  withFocusInDeclared,
  type DevSet,
  type DevWeek,
} from './simulate';

describe('buildQuery', () => {
  it('設定をそのまま渡す', () => {
    const q = new URLSearchParams(buildQuery(defaultForm));
    expect(q.get('declared')).toBe('bench,squat,deadlift');
    expect(q.get('focus')).toBe('bench');
    expect(q.get('split')).toBe('upper_lower');
    expect(q.get('frequency')).toBe('4');
    expect(q.get('weeks')).toBe('4');
  });

  // 「分割なし・重点なし」は空で送る。キーごと落とすと、URL を読み戻した
  // ときに既定（上下分割・ベンチ）に戻り、「なし」の状況を URL で再現
  // できない。サーバーは空を指定なしとして扱う（dev_simulation_test.go）。
  it('分割と重点の「なし」は空のまま送る', () => {
    const q = new URLSearchParams(buildQuery({ ...defaultForm, focus: '', split: '' }));
    expect(q.get('focus')).toBe('');
    expect(q.get('split')).toBe('');
  });

  // 模擬ユーザーの既定値はサーバーが持つ。画面が同じ定数を二重に持たない
  // ように、触っていない項目は送らずサーバーに任せる。
  it('模擬ユーザーは触った項目だけ送る', () => {
    const q = new URLSearchParams(buildQuery(defaultForm));
    for (const k of ['growth', 'first_pct', 'body_weight', 'orm', 'exercises', 'sets', 'days', 'start']) {
      expect(q.has(k)).toBe(false);
    }

    const got = new URLSearchParams(
      buildQuery({ ...defaultForm, growth: 0, firstPct: 60, bodyWeight: 82, orm: { squat: 150, bench: 110 } }),
    );
    // 0 は「伸びない」という設定で、未指定ではない。
    expect(got.get('growth')).toBe('0');
    expect(got.get('first_pct')).toBe('60');
    expect(got.get('body_weight')).toBe('82');
    // 並びを固定する。同じ設定が同じ URL になる。
    expect(got.get('orm')).toBe('bench:110,squat:150');
  });
});

describe('buildQuery（予定）', () => {
  // 曜日を指定したら頻度はその数。食い違って送るとサーバーが 400 を返す。
  it('曜日を送るときは、頻度を曜日の数に揃える', () => {
    const q = new URLSearchParams(buildQuery({ ...defaultForm, frequency: 4, days: [1, 3] }));
    expect(q.get('days')).toBe('1,3');
    expect(q.get('frequency')).toBe('2');
  });

  it('量と開始日は触ったときだけ送る', () => {
    const q = new URLSearchParams(buildQuery({ ...defaultForm, exercises: 5, sets: 4, start: '2026-09-07' }));
    expect(q.get('exercises')).toBe('5');
    expect(q.get('sets')).toBe('4');
    expect(q.get('start')).toBe('2026-09-07');
  });
});

describe('toggleDay', () => {
  const defaults = { '2': [0, 3], '3': [0, 2, 4] };

  // 未指定（既定の曜日）から1つ外すと、既定を起点に外した曜日が残る。
  // 空から始めると、既定の曜日が全部外れて1日だけになる。
  it('既定の曜日を起点に切り替え、頻度をその数にする', () => {
    const got = toggleDay({ ...defaultForm, frequency: 3, days: null }, 2, defaults);
    expect(got.days).toEqual([0, 4]);
    expect(got.frequency).toBe(2);
  });

  it('足した曜日は並べて持つ', () => {
    const got = toggleDay({ ...defaultForm, frequency: 2, days: [0, 3] }, 1, defaults);
    expect(got.days).toEqual([0, 1, 3]);
    expect(got.frequency).toBe(3);
  });

  // 0日は頻度として成り立たない。最後の1日は外せない。
  it('最後の1日は外さない', () => {
    const form = { ...defaultForm, frequency: 1, days: [4] };
    expect(toggleDay(form, 4, defaults)).toBe(form);
  });
});

describe('parseForm', () => {
  // URL を開けば同じ状況が再現できること。Claude が URL 1本で読みに来る。
  it('buildQuery の出力を読み戻すと同じ設定になる', () => {
    const form = {
      ...defaultForm,
      declared: ['squat', 'pull_up'],
      focus: 'squat',
      split: '',
      frequency: 3,
      weeks: 12,
      growth: -1.5,
      firstPct: 55,
      bodyWeight: 68,
      orm: { pull_up: 0, squat: 160 },
      exercises: 5,
      sets: 2,
      days: [1, 5, 6],
      start: '2026-09-07',
    };
    form.frequency = 3;
    expect(parseForm(buildQuery(form), defaultForm)).toEqual(form);
  });

  it('何も無ければ既定のまま', () => {
    expect(parseForm('', defaultForm)).toEqual(defaultForm);
  });

  // 数字でないものは読み飛ばして既定に倒す。範囲はサーバーが見る。
  it('読めない値は既定に倒す', () => {
    const got = parseForm('frequency=x&weeks=&growth=fast&orm=bench:heavy,squat:150,nope', defaultForm);
    expect(got.frequency).toBe(defaultForm.frequency);
    expect(got.weeks).toBe(defaultForm.weeks);
    expect(got.growth).toBeNull();
    expect(got.orm).toEqual({ squat: 150 });
  });

  // 分割を「指定しない」は空文字で表す。URL に split が無いときの既定
  // （上下分割）と区別しないと、分割なしの状況を URL で再現できない。
  it('空の split は「分割なし」として読む', () => {
    expect(parseForm('split=', defaultForm).split).toBe('');
  });
});

describe('setOneRepMax', () => {
  it('既定値と違えば上書きとして持つ', () => {
    expect(setOneRepMax({}, 'bench', 120, 100)).toEqual({ bench: 120 });
  });

  // 既定値に戻したら上書きを消す。残すと URL に既定値が並び、どれを
  // 変えたのかが読めなくなる。
  it('既定値に戻したら上書きを消す', () => {
    expect(setOneRepMax({ bench: 120, squat: 150 }, 'bench', 100, 100)).toEqual({ squat: 150 });
  });

  it('数字でなければ上書きを消す', () => {
    expect(setOneRepMax({ bench: 120 }, 'bench', Number.NaN, 100)).toEqual({});
  });
});

describe('curlCommand', () => {
  // 画面を通さずに JSON を読むための1行。トークンは環境変数のまま出す
  // （画面に値を書き出さない）。
  it('同じ設定の API を叩く1行を返す', () => {
    const got = curlCommand('http://localhost:8080', defaultForm);
    expect(got).toContain(`'http://localhost:8080/api/dev/simulate?${buildQuery(defaultForm)}'`);
    expect(got).toContain('Authorization: Bearer $LIFTPLAN_TOKEN');
  });
});

describe('toggle', () => {
  it('入っていなければ足し、入っていれば外す', () => {
    expect(toggle(['bench'], 'squat')).toEqual(['bench', 'squat']);
    expect(toggle(['bench', 'squat'], 'bench')).toEqual(['squat']);
  });
});

describe('withFocusInDeclared', () => {
  // 宣言から外した種目を重点に残したまま送ると 400 が返る。
  it('重点種目が宣言に無ければ指定を落とす', () => {
    const got = withFocusInDeclared({ ...defaultForm, declared: ['squat'], focus: 'bench' });
    expect(got.focus).toBe('');
  });

  it('宣言に入っていればそのまま', () => {
    expect(withFocusInDeclared(defaultForm).focus).toBe('bench');
  });
});

describe('tone', () => {
  // 許容は 60〜145%。境界がずれると、通し検証が緑なのに画面が赤くなる。
  it.each([
    [0.59, 'low'],
    [0.6, 'ok'],
    [1.45, 'ok'],
    [1.46, 'high'],
  ])('%s は %s', (value, want) => {
    expect(tone(value)).toBe(want);
  });
});

describe('rate', () => {
  it('目標が0なら0。0除算を画面に出さない', () => {
    expect(rate(3, 0)).toBe(0);
    expect(rate(6, 12)).toBe(0.5);
  });
});

describe('outOfRange', () => {
  it('許容から外れた区分だけを返す', () => {
    const week: DevWeek = {
      index: 1,
      regions: [
        { region: 'CHEST_MID', target: 12, done: 12 },
        { region: 'CALF', target: 10, done: 3 },
        { region: 'ABS', target: 10, done: 20 },
      ],
    };
    expect(outOfRange(week).map((r) => r.region)).toEqual(['CALF', 'ABS']);
  });
});

describe('表示', () => {
  const set = (over: Partial<DevSet>): DevSet => ({
    exercise_id: 'bench',
    name: 'ベンチプレス',
    weight_kg: 95,
    sets: 3,
    target_rir: 1,
    pct_of_1rm: 0.88,
    athlete_1rm_kg: 100,
    performed: { weight_kg: 95, reps: 3, rir: 1 },
    ...over,
  });

  it('重量が未確定なら、本人が決める枠だと分かる形で出す', () => {
    expect(formatWeight(set({ weight_kg: null }))).toBe('自分で決める');
    expect(formatWeight(set({}))).toBe('95kg');
    // 自重だけで強度を超える回は加重0が処方される。記録の表記と揃える。
    expect(formatWeight(set({ weight_kg: 0 }))).toBe('自重');
  });

  it('記録は「重量×回 RIR」。自重だけなら自重と出す', () => {
    expect(formatPerformed(set({}))).toBe('95kg×3 RIR1');
    expect(formatPerformed(set({ performed: { weight_kg: 0, reps: 9, rir: 2 } }))).toBe('自重×9 RIR2');
  });

  it('推定1RMに対する比は小数2桁。立っていなければ空', () => {
    expect(formatPct(set({}))).toBe('0.88');
    expect(formatPct(set({ pct_of_1rm: null }))).toBe('');
  });
});

describe('describeFailure', () => {
  // 401 だけは「もう一度押す」で直らない。画面が案内を変える。
  it('認証が切れていたらログインし直す先を出す', () => {
    const got = describeFailure(new Unauthorized());
    expect(got).toContain('ログイン');
    expect(got).not.toBe('unauthorized');
  });

  // サーバーは入力が成り立たない理由を本文に書いて 400 を返す。
  // 握り潰すと、画面には「400 を返した」しか出ない。
  it('サーバーが書いた理由はそのまま出す', () => {
    expect(describeFailure(new Error('クエリ weeks が不正である: zero'))).toBe(
      'クエリ weeks が不正である: zero',
    );
  });

  it('Error でないものも文字列にする', () => {
    expect(describeFailure('落ちた')).toBe('落ちた');
  });
});

describe('custom（自分の種目）', () => {
  const custom =
    'アイソラテラル・ロー|TRAP_MID:1,LAT:0.5,BICEPS:0.5|2.5;アイソラテラル・フロント・プルダウン|LAT:1|2.5';

  // 自分の種目はサーバーと同じ1行の書式のまま URL に載せる。画面で組み直すと、
  // URL を開き直したときに書式の違いで別の条件になる。
  it('空なら送らず、書いたらそのまま送る', () => {
    expect(new URLSearchParams(buildQuery(defaultForm)).has('custom')).toBe(false);
    const q = new URLSearchParams(buildQuery({ ...defaultForm, custom: `  ${custom}  ` }));
    expect(q.get('custom')).toBe(custom);
  });

  it('URL から読み戻せる', () => {
    const search = buildQuery({ ...defaultForm, custom });
    expect(parseForm(search, defaultForm).custom).toBe(custom);
    expect(parseForm('', defaultForm).custom).toBe('');
  });
});
