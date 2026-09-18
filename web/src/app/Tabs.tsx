import { Button } from '../ui/Button';

export type View = 'today' | 'history';

const TABS: { id: View; label: string }[] = [
  { id: 'today', label: '今日' },
  { id: 'history', label: '履歴' },
];

// Tabs は画面の行き来。
//
// **設定のタブは作らなかった。**プログラムの設定は `ProgramSettings` として
// 既に「今日」の末尾に畳んである。タブに切り出すと、同じものへの入口が
// 2つになるか、いま動いているものを別の画面へ移すことになる。増えたのは
// 履歴の1つだけなので、タブも1つだけ増やす（D-127）。
//
// 上に置く。下端は状態バーと休憩タイマーが二段で占めていて、そこへ足すと
// ジムで一番見たい「記録が送られたか」が押し出される。
//
// 見た目は ui/Button に寄せる。タブは押せるものなので、押せるものの
// 見た目がここだけ別にならないようにする。選んでいる側が quiet（枠付き）、
// 選んでいない側が ghost（文字だけ）。
//
// `selected` は使わない。タブは1つしか立たないので、複数立ちうる
// aria-pressed ではなく aria-selected で表す。
export function Tabs({ view, onChange }: { view: View; onChange: (v: View) => void }) {
  return (
    <div className="flex gap-1 px-3 pb-2.5" role="tablist">
      {TABS.map((t) => (
        <Button
          key={t.id}
          variant={view === t.id ? 'quiet' : 'ghost'}
          size="md"
          role="tab"
          aria-selected={view === t.id}
          onClick={() => onChange(t.id)}
          className="flex-1 font-medium"
        >
          {t.label}
        </Button>
      ))}
    </div>
  );
}
