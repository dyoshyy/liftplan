import { describe, expect, it } from 'vitest';
import type { Exercise } from '../../api/types';
import { pickableExercises } from './pickable';

const ex = (id: string, name: string): Exercise => ({ id, name, increment_kg: 2.5, stimulus: {} });

const all = [
  ex('bench', 'ベンチプレス'),
  ex('squat', 'スクワット'),
  ex('dip', 'ディップス'),
  ex('curl', 'バーベルカール'),
];

const none = new Set<string>();

describe('pickableExercises', () => {
  // 使わない種目は計画にも出ないので、選んで記録のシートにも出さない。
  it('使う種目に入っていない種目は選択肢に入れない', () => {
    expect(pickableExercises(all, none, '', ['bench', 'curl']).map((e) => e.id)).toEqual(['bench', 'curl']);
  });

  // 使う種目が読めていない間に一覧を空にすると、何も選べなくなる。
  it('使う種目が読めていない（null）ときは絞らない', () => {
    expect(pickableExercises(all, none, '', null)).toHaveLength(4);
  });

  // 今日の画面なら予定に出ている種目、履歴ならその日に記録がある種目。
  // 選んでも同じ種目が2つ並ぶだけで、既にあるほうに足せば済む。
  // 消した種目は使う種目からも外れるが、使う種目が読めていない（null）ときは
  // 絞らないので、それだけでは混ざる。消した種目は記録の対象にしない。
  it('消した種目は、使う種目が読めていなくても選択肢に入れない', () => {
    const withDeleted = [...all, { ...ex('fly', 'フライ'), deleted: true }];
    expect(pickableExercises(withDeleted, none, '', null).map((e) => e.id)).not.toContain('fly');
  });

  it('除く種目は選択肢に入れない', () => {
    const got = pickableExercises(all, new Set(['bench', 'dip']), '', null);
    expect(got.map((e) => e.id)).toEqual(['squat', 'curl']);
  });

  it('名前の一部で絞る', () => {
    expect(pickableExercises(all, none, 'カール', null).map((e) => e.id)).toEqual(['curl']);
  });

  // 前後の空白は入力の癖であって、種目名の一部ではない。
  it('前後の空白は無視する', () => {
    expect(pickableExercises(all, none, '  ベンチ ', null).map((e) => e.id)).toEqual(['bench']);
  });

  // 名前の側とクエリの側、両方を揃えて比べる。片方だけ揃えると、大文字で
  // 打った／大文字で登録した、のどちらかが見つからなくなる。
  it('英字は大文字小文字を区別しない', () => {
    const list = [ex('ohp', 'OHP'), ex('bench', 'ベンチプレス')];
    expect(pickableExercises(list, none, 'ohp', null).map((e) => e.id)).toEqual(['ohp']);
    expect(pickableExercises(list, none, 'OHP', null).map((e) => e.id)).toEqual(['ohp']);
    expect(pickableExercises([ex('db', 'db press')], none, 'DB', null).map((e) => e.id)).toEqual(['db']);
  });

  it('サーバーが返した並びを崩さない', () => {
    expect(pickableExercises(all, none, '', null).map((e) => e.id)).toEqual([
      'bench',
      'squat',
      'dip',
      'curl',
    ]);
  });

  it('一致するものが無ければ空', () => {
    expect(pickableExercises(all, none, '存在しない', null)).toEqual([]);
  });
});
