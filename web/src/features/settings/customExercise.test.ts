import { describe, expect, it } from 'vitest';
import type { Exercise } from '../../api/types';
import {
  aliveExercises,
  cycleRole,
  deleteBlockedReason,
  draftBody,
  draftProblem,
  emptyDraft,
  type CustomExerciseDraft,
} from './customExercise';

const ex = (id: string, extra: Partial<Exercise> = {}): Exercise => ({
  id,
  name: id,
  increment_kg: 2.5,
  stimulus: { LAT: 1 },
  ...extra,
});

const withRoles = (roles: CustomExerciseDraft['roles'], name = 'アイソラテラル・ロー'): CustomExerciseDraft => ({
  ...emptyDraft(),
  name,
  roles,
});

describe('cycleRole', () => {
  // 1つのチップを押すたびに 無し → 主 → 少し → 無し と回る。2つの一覧に
  // 分けないのは、同じ部位を主と少しの両方に入れる操作をそもそも作らないため
  // （サーバーは重なりを 400 で弾く）。
  it('無し → 主 → 少し → 無し と回る', () => {
    const a = cycleRole(emptyDraft(), 'LAT');
    expect(a.roles).toEqual({ LAT: 'primary' });
    const b = cycleRole(a, 'LAT');
    expect(b.roles).toEqual({ LAT: 'secondary' });
    const c = cycleRole(b, 'LAT');
    expect(c.roles).toEqual({});
  });

  it('元の下書きを書き換えない', () => {
    const a = emptyDraft();
    cycleRole(a, 'LAT');
    expect(a.roles).toEqual({});
  });
});

describe('draftProblem', () => {
  it('名前と主に効く部位があれば送れる', () => {
    expect(draftProblem(withRoles({ TRAP_MID: 'primary', LAT: 'secondary' }))).toBeNull();
  });

  // 主に効く部位が無い種目は、分割のどの日にも入らない（サーバーの isPrimaryIn）。
  // 送る前に止めないと、足した種目が一度も出てこない理由が本人に分からない。
  it('主に効く部位が無ければ止める', () => {
    expect(draftProblem(withRoles({ LAT: 'secondary' }))).toMatch(/主に効く/);
  });

  it('名前が空なら止める（前後の空白だけも空）', () => {
    expect(draftProblem(withRoles({ LAT: 'primary' }, '   '))).toMatch(/名前/);
  });

  it('名前は40文字まで', () => {
    expect(draftProblem(withRoles({ LAT: 'primary' }, 'あ'.repeat(40)))).toBeNull();
    expect(draftProblem(withRoles({ LAT: 'primary' }, 'あ'.repeat(41)))).toMatch(/40/);
  });

  it('部位は合わせて8つまで', () => {
    const regions = ['LAT', 'TRAP_MID', 'TRAP_UPPER', 'ERECTOR', 'BICEPS', 'FOREARM', 'REAR_DELT', 'SIDE_DELT', 'ABS'];
    const roles = Object.fromEntries(regions.map((r, i) => [r, i === 0 ? 'primary' : 'secondary'])) as CustomExerciseDraft['roles'];
    expect(draftProblem(withRoles(roles))).toMatch(/8/);
    delete roles.ABS;
    expect(draftProblem(withRoles(roles))).toBeNull();
  });

  it('刻みは0より大きく50kg以下', () => {
    expect(draftProblem({ ...withRoles({ LAT: 'primary' }), incrementKg: 0 })).toMatch(/刻み/);
    expect(draftProblem({ ...withRoles({ LAT: 'primary' }), incrementKg: 51 })).toMatch(/刻み/);
  });
});

describe('draftBody', () => {
  // サーバーの addCustomExerciseDTO と対。区分は並べて送る（同じ下書きから
  // 毎回同じ本文ができるように）。
  it('主と少しに分け、区分を並べ、名前の前後の空白を落とす', () => {
    const body = draftBody({
      name: '  アイソラテラル・ロー ',
      roles: { TRAP_MID: 'primary', LAT: 'secondary', BICEPS: 'secondary' },
      incrementKg: 2.5,
    });
    expect(body).toEqual({
      name: 'アイソラテラル・ロー',
      primary: ['TRAP_MID'],
      secondary: ['BICEPS', 'LAT'],
      increment_kg: 2.5,
    });
  });
});

describe('aliveExercises', () => {
  // 消した種目もサーバーは返す（履歴の名前のため）。設定の一覧に出すと、
  // 消したはずの種目がまた選べてしまい、選ぶと 400 で断られる。
  it('消した種目を落とす', () => {
    const got = aliveExercises([ex('bench'), ex('u-1', { custom: true, deleted: true }), ex('u-2', { custom: true })]);
    expect(got.map((e) => e.id)).toEqual(['bench', 'u-2']);
  });
});

describe('deleteBlockedReason', () => {
  // サーバーも 409 で断るが、押す前に理由が読めたほうが次に何をすればいいか
  // 分かる（分割の頻度下限と同じ扱い）。
  it('伸ばしたい種目に入っていれば理由を返す', () => {
    expect(deleteBlockedReason(['u-1'], 'u-1')).toMatch(/伸ばしたい種目/);
    expect(deleteBlockedReason(['bench'], 'u-1')).toBeNull();
  });
});
