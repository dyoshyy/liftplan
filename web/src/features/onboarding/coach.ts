// 初回の設定のあとに「今日」の画面で流す使い方の案内。
//
// 指す場所は data-coach 属性で探す（CoachMarks.tsx）。ここはどの段階を
// どの順で出すかだけを決める。

/** CoachTarget は指す要素の data-coach の値。 */
export type CoachTarget = 'target' | 'slot' | 'lane' | 'rest' | 'settings';

export type CoachStep = { target: CoachTarget; text: string };

/** COACH_STEPS は案内の段階。この順に出す。 */
export const COACH_STEPS: readonly CoachStep[] = [
  {
    target: 'target',
    text: '重さはアプリが決めます。初めての種目は「自分で決める」と出るので、軽めから始めてください。記録すれば次から出ます。',
  },
  {
    target: 'slot',
    text: 'セットが終わったらここを押して、重さ・回数・RIR（あと何回できたか）を記録します。',
  },
  {
    target: 'lane',
    text: '上から順にやって、好きなところでやめて大丈夫です。',
  },
  {
    target: 'rest',
    text: '休憩の時間を計れます。記録すると自動で始まります。',
  },
  {
    target: 'settings',
    text: '通い方はここでいつでも変えられます。',
  },
];

/**
 * nextCoachIndex は from 以降で、指す要素がある最初の段階を返す。
 * 無ければ null（案内は終わり）。
 *
 * 無い要素を指すと、暗い画面に吹き出しだけが浮いて何も説明できない。
 */
export function nextCoachIndex(
  steps: readonly CoachStep[],
  from: number,
  present: (target: CoachTarget) => boolean,
): number | null {
  for (let i = from; i < steps.length; i++) {
    const s = steps[i];
    if (s && present(s.target)) return i;
  }
  return null;
}
