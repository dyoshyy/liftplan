export type View = 'today' | 'history' | 'settings';

const TABS: { id: View; label: string }[] = [
  { id: 'today', label: '今日' },
  { id: 'history', label: '履歴' },
  { id: 'settings', label: '設定' },
];

export function Tabs({ view, onChange }: { view: View; onChange: (v: View) => void }) {
  return (
    <div className="flex gap-1 px-3 pb-2.5" role="tablist">
      {TABS.map((t) => (
        <button
          key={t.id}
          type="button"
          role="tab"
          aria-selected={view === t.id}
          onClick={() => onChange(t.id)}
          className={`flex-1 rounded-[10px] py-[9px] text-center text-sm font-medium ${
            view === t.id ? 'bg-surface-2 text-text' : 'text-muted'
          }`}
        >
          {t.label}
        </button>
      ))}
    </div>
  );
}
