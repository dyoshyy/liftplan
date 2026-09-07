// 筋区分の日本語。表示の都合なのでここに置く。ドメインに持たせると、
// 画面の言語がドメインに漏れる。
const REGION: Record<string, string> = {
  CHEST_UPPER: '胸（上部）',
  CHEST_MID: '胸（中部）',
  CHEST_LOWER: '胸（下部）',
  LAT: '広背筋',
  TRAP_MID: '僧帽筋（中部）',
  TRAP_UPPER: '僧帽筋（上部）',
  ERECTOR: '脊柱起立筋',
  FRONT_DELT: '三角筋（前）',
  SIDE_DELT: '三角筋（横）',
  REAR_DELT: '三角筋（後）',
  TRICEPS_LONG: '三頭（長頭）',
  TRICEPS_LATERAL: '三頭（外側）',
  BICEPS: '二頭',
  FOREARM: '前腕',
  QUAD: '大腿四頭筋',
  HAMSTRING: 'ハムストリング',
  GLUTE: '臀筋',
  ADDUCTOR: '内転筋',
  CALF: 'ふくらはぎ',
  ABS: '腹直筋',
  OBLIQUE: '腹斜筋',
};

export const regionName = (r: string): string => REGION[r] ?? r;
