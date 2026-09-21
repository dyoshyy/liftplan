import {
  declaredCandidates,
  formatPct,
  formatWeight,
  outOfRange,
  rate,
  toggle,
  tone,
  type DevDay,
  type DevSet,
  type DevWeek,
} from './simulate';
import { useSimulation } from './useSimulation';

// シミュレーション画面。
//
// 設定を変えて1ヶ月ぶんの計画を作り、何が起きているかを目で見る。
// 通し検証（seed のテスト）が数字で守るのに対して、こちらは形を見せる。
//
// 本番のバンドルにも入る（/dev.html）。メインの画面からは辿れない。
// 導線を付けないのは、ここで変えたものが何も保存されないため。設定を
// 変える場所は設定画面1つに保つ。
export function DevSimulation() {
  const { options, form, setForm, result, error, busy, run } = useSimulation();

  return (
    <div className="mx-auto grid max-w-[900px] gap-4 p-4">
      <header className="flex items-baseline gap-3">
        <h1 className="num text-[19px] font-semibold uppercase tracking-[0.08em]">
          lift<span className="text-amber">plan</span> / sim
        </h1>
        <span className="text-[13px] text-muted">記録は保存されない</span>
      </header>

      <section className="grid gap-3 rounded-[14px] border border-line bg-surface p-4">
        <Row label="伸ばしたい種目">
          <div className="flex flex-wrap gap-1.5">
            {declaredCandidates(options?.exercises ?? []).map((e) => (
              <button
                key={e.id}
                type="button"
                onClick={() => setForm({ ...form, declared: toggle(form.declared, e.id) })}
                className={`rounded-full border px-2.5 py-1 text-[12px] ${
                  form.declared.includes(e.id)
                    ? 'border-amber text-amber'
                    : 'border-line text-muted'
                }`}
              >
                {e.name}
                {e.derived_from && <span className="ml-1 opacity-60">派生</span>}
              </button>
            ))}
          </div>
        </Row>

        <Row label="重点種目">
          <select
            id="focus"
            value={form.focus}
            onChange={(ev) => setForm({ ...form, focus: ev.target.value })}
            className="rounded-md border border-line bg-ground px-2 py-1 text-[13px]"
          >
            <option value="">指定しない</option>
            {form.declared.map((id) => (
              <option key={id} value={id}>
                {options?.exercises.find((e) => e.id === id)?.name ?? id}
              </option>
            ))}
          </select>
        </Row>

        <Row label="分割">
          <select
            id="split"
            value={form.split}
            onChange={(ev) => setForm({ ...form, split: ev.target.value })}
            className="rounded-md border border-line bg-ground px-2 py-1 text-[13px]"
          >
            <option value="">指定しない</option>
            {options?.presets.map((p) => (
              <option key={p.key} value={p.key}>
                {p.name}（{p.days.join(' → ')}）
              </option>
            ))}
          </select>
        </Row>

        <Row label="週の回数 / 期間">
          <div className="flex items-center gap-2">
            <select
              id="frequency"
              value={form.frequency}
              onChange={(ev) => setForm({ ...form, frequency: Number(ev.target.value) })}
              className="rounded-md border border-line bg-ground px-2 py-1 text-[13px]"
            >
              {[1, 2, 3, 4, 5, 6, 7].map((n) => (
                <option key={n} value={n}>
                  週{n}
                </option>
              ))}
            </select>
            <select
              id="weeks"
              value={form.weeks}
              onChange={(ev) => setForm({ ...form, weeks: Number(ev.target.value) })}
              className="rounded-md border border-line bg-ground px-2 py-1 text-[13px]"
            >
              {[1, 2, 4, 8, 12].map((n) => (
                <option key={n} value={n}>
                  {n}週
                </option>
              ))}
            </select>
            <button
              type="button"
              disabled={busy || form.declared.length === 0}
              onClick={run}
              className="rounded-md bg-amber px-3 py-1.5 text-[13px] font-bold text-ground disabled:opacity-40"
            >
              {busy ? '作成中' : '作る'}
            </button>
          </div>
        </Row>
      </section>

      {error && (
        <p className="rounded-[14px] border border-red bg-surface p-3 text-[13px] text-red">
          {error}
        </p>
      )}

      {result && (
        <>
          <section className="grid gap-2">
            {result.weeks.map((w) => (
              <WeekRow key={w.index} week={w} />
            ))}
          </section>

          <section className="grid gap-2">
            {result.days.map((d) => (
              <DayCard key={d.date} day={d} />
            ))}
          </section>
        </>
      )}
    </div>
  );
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="grid gap-1.5">
      <div className="text-[12px] font-bold text-muted">{label}</div>
      {children}
    </div>
  );
}

function WeekRow({ week }: { week: DevWeek }) {
  const bad = outOfRange(week);
  return (
    <div className="rounded-[14px] border border-line bg-surface p-3">
      <div className="mb-2 flex items-baseline gap-2">
        <span className="text-[13px] font-bold">{week.index}週目</span>
        <span className="text-[12px] text-muted">
          {bad.length === 0 ? '全区分が 60〜145% に収まっている' : `範囲外 ${bad.length} 区分`}
        </span>
      </div>
      <div className="flex flex-wrap gap-1.5">
        {week.regions.map((r) => {
          const value = rate(r.done, r.target);
          const t = tone(value);
          return (
            <span
              key={r.region}
              title={`目標 ${r.target} / 実測 ${r.done.toFixed(1)}`}
              className={`num rounded border px-1.5 py-0.5 text-[11px] ${
                t === 'ok' ? 'border-line text-muted' : 'border-red text-red'
              }`}
            >
              {r.region} {Math.round(value * 100)}%
            </span>
          );
        })}
      </div>
    </div>
  );
}

function DayCard({ day }: { day: DevDay }) {
  return (
    <div className="rounded-[14px] border border-line bg-surface p-3">
      <div className="mb-2 flex items-baseline gap-2">
        <span className="num text-[13px] font-bold">{day.date}</span>
        {day.split && <span className="text-[12px] text-amber">{day.split}</span>}
        <span className="ml-auto num text-[12px] text-muted">{day.total_sets}セット</span>
      </div>
      <Lane name="軸" sets={day.main} />
      <Lane name="バリエーション" sets={day.variation} />
      <Lane name="補助" sets={day.accessories} />
    </div>
  );
}

function Lane({ name, sets }: { name: string; sets: DevSet[] }) {
  if (sets.length === 0) return null;
  return (
    <div className="mb-1.5 grid grid-cols-[6.5em_1fr] gap-2">
      <div className="text-[12px] text-muted">{name}</div>
      <div className="grid gap-0.5">
        {sets.map((s) => (
          <div key={s.exercise_id} className="flex flex-wrap items-baseline gap-x-2 text-[13px]">
            <span>{s.name}</span>
            <span className="num text-muted">{formatWeight(s)}</span>
            <span className="num text-muted">
              ×{s.sets}セット RIR{s.target_rir}
            </span>
            {formatPct(s) && <span className="num text-[11px] text-amber">{formatPct(s)}</span>}
          </div>
        ))}
      </div>
    </div>
  );
}
