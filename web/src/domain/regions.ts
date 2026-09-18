/**
 * 筋区分の表示名。サーバーの training.MuscleRegion と対。
 *
 * サーバーが日本語を持たないのは、名前が表示の都合だから。ドメインは
 * 区分の同一性だけを扱う。
 *
 * ここに無い区分は、そのままコードを出す。サーバーが区分を足したときに
 * 画面が落ちるより、英語のまま出るほうがよい。
 */
const LABELS: Record<string, string> = {
  CHEST_UPPER: '大胸筋上部',
  CHEST_MID: '大胸筋中部',
  CHEST_LOWER: '大胸筋下部',
  LAT: '広背筋',
  TRAP_MID: '僧帽筋中部',
  TRAP_UPPER: '僧帽筋上部',
  ERECTOR: '脊柱起立筋',
  FRONT_DELT: '三角筋前部',
  SIDE_DELT: '三角筋中部',
  REAR_DELT: '三角筋後部',
  TRICEPS_LONG: '上腕三頭筋長頭',
  TRICEPS_LATERAL: '上腕三頭筋外側頭',
  BICEPS: '上腕二頭筋',
  FOREARM: '前腕',
  QUAD: '大腿四頭筋',
  HAMSTRING: 'ハムストリング',
  GLUTE: '臀筋',
  ADDUCTOR: '内転筋',
  CALF: 'ふくらはぎ',
  ABS: '腹直筋',
  OBLIQUE: '腹斜筋',
};

export function regionLabel(region: string): string {
  return LABELS[region] ?? region;
}
