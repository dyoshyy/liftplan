import { API_BASE } from '../api/client';
import { weightSeries } from './chart';
import { RegionHeatmap } from './RegionHeatmap';
import {
  buildQuery,
  curlCommand,
  declaredCandidates,
  formatPct,
  formatPerformed,
  formatWeight,
  setOneRepMax,
  toggle,
  toggleDay,
  type DevDay,
  type DevExercise,
  type DevOptions,
  type DevResult,
  type DevSet,
  type Form,
} from './simulate';
import { liftLine, settingsLine, weekdayName, weekLine } from './summary';
import { useSimulation } from './useSimulation';
import { WeightTrend } from './WeightTrend';

// シミュレーション画面。
//
// 設定を変えて計画を作り、何が起きているかを見る。通し検証（seed の
// テスト）が数字で守るのに対して、こちらは形を見せる。
//
// **PC で開く開発用の道具で、Claude も読む。**スマホ向けの配慮はしない。
// 代わりに、数字はホバーや折りたたみに隠さず文字で出し、設定は URL に
// 載せる（URL を開けば同じ結果が出る）。結果の先頭の「要約」は、画面の
// 文字を読んだ Claude がまずそこだけで判断できるように書いてある。
//
// 本番のバンドルにも入る（/dev.html）。メインの画面からは辿れない。
// 導線を付けないのは、ここで変えたものが何も保存されないため。設定を
// 変える場所は設定画面1つに保つ。
export function DevSimulation() {
  const { options, form, setForm, result, ranForm, error, busy, run } = useSimulation();

  return (
    <div className="mx-auto grid max-w-[1280px] gap-5 p-6">
      <header className="flex items-baseline gap-3">
        <h1 className="num text-[19px] font-semibold uppercase tracking-[0.08em]">
          lift<span className="text-amber">plan</span> / sim
        </h1>
        <span className="text-[13px] text-muted">記録は保存されない。設定は URL に載る</span>
      </header>

      <Settings options={options} form={form} setForm={setForm} busy={busy} run={run} />

      {error && (
        <p role="alert" className="rounded-[14px] border border-red bg-surface p-3 text-[13px] text-red">
          {error}
        </p>
      )}

      {result && ranForm && <Results options={options} result={result} ranForm={ranForm} />}
    </div>
  );
}

function Settings({
  options,
  form,
  setForm,
  busy,
  run,
}: {
  options: DevOptions | null;
  form: Form;
  setForm: (f: Form) => void;
  busy: boolean;
  run: () => void;
}) {
  const nameOf = (id: string) => options?.exercises.find((e) => e.id === id)?.name ?? id;
  const defaults = options?.athlete_defaults;

  return (
    <section className="grid gap-4 rounded-[14px] border border-line bg-surface p-4" aria-label="設定">
      <Row label="伸ばしたい種目">
        <div className="flex flex-wrap gap-1.5">
          {declaredCandidates(options?.exercises ?? []).map((e) => (
            <button
              key={e.id}
              type="button"
              aria-pressed={form.declared.includes(e.id)}
              onClick={() => setForm({ ...form, declared: toggle(form.declared, e.id) })}
              className={`rounded-full border px-2.5 py-1 text-[12px] ${
                form.declared.includes(e.id) ? 'border-amber text-amber' : 'border-line text-muted'
              }`}
            >
              {e.name}
              {e.derived_from && <span className="ml-1 opacity-60">派生</span>}
            </button>
          ))}
        </div>
      </Row>

      <div className="grid grid-cols-4 gap-4">
        <Row label="重点種目">
          <select
            id="focus"
            value={form.focus}
            onChange={(ev) => setForm({ ...form, focus: ev.target.value })}
            className={FIELD}
          >
            <option value="">指定しない</option>
            {form.declared.map((id) => (
              <option key={id} value={id}>
                {nameOf(id)}
              </option>
            ))}
          </select>
        </Row>

        <Row label="分割">
          <select
            id="split"
            value={form.split}
            onChange={(ev) => setForm({ ...form, split: ev.target.value })}
            className={FIELD}
          >
            <option value="">指定しない</option>
            {options?.presets.map((p) => (
              <option key={p.key} value={p.key}>
                {p.name}（{p.days.join(' → ')}）
              </option>
            ))}
          </select>
        </Row>

        <Row label="週の回数">
          <select
            id="frequency"
            value={form.frequency}
            // 頻度を選び直したら、曜日は頻度ごとの既定に戻す。
            onChange={(ev) => setForm({ ...form, frequency: Number(ev.target.value), days: null })}
            className={FIELD}
          >
            {[1, 2, 3, 4, 5, 6, 7].map((n) => (
              <option key={n} value={n}>
                週{n}
              </option>
            ))}
          </select>
        </Row>

        <Row label="期間">
          <select
            id="weeks"
            value={form.weeks}
            onChange={(ev) => setForm({ ...form, weeks: Number(ev.target.value) })}
            className={FIELD}
          >
            {[1, 2, 4, 8, 12].map((n) => (
              <option key={n} value={n}>
                {n}週
              </option>
            ))}
          </select>
        </Row>
      </div>

      <div className="grid grid-cols-4 gap-4">
        <Row label="通う曜日（押すと頻度もその数になる）">
          <Weekdays options={options} form={form} setForm={setForm} />
        </Row>
        <NumberField
          id="exercises"
          label="1回の種目数"
          step={1}
          value={form.exercises}
          fallback={options?.schedule_defaults.exercises_per_session}
          onChange={(v) => setForm({ ...form, exercises: v })}
        />
        <NumberField
          id="sets"
          label="1種目のセット数"
          step={1}
          value={form.sets}
          fallback={options?.schedule_defaults.sets_per_exercise}
          onChange={(v) => setForm({ ...form, sets: v })}
        />
        <label htmlFor="start" className="grid gap-1.5">
          <span className="text-[12px] font-bold text-muted">開始日</span>
          <input
            id="start"
            type="date"
            value={form.start ?? options?.schedule_defaults.start ?? ''}
            onChange={(ev) => setForm({ ...form, start: ev.target.value || null })}
            className={`num ${FIELD}`}
          />
        </label>
      </div>

      <div className="grid gap-3 border-t border-line-soft pt-4">
        <h2 className="text-[13px] font-bold">模擬ユーザー（処方をこなす本人）</h2>
        <div className="grid grid-cols-4 gap-4">
          <NumberField
            id="growth"
            label="実力の伸び（%/週）"
            step={0.1}
            value={form.growth}
            fallback={defaults?.growth_pct_per_week}
            onChange={(v) => setForm({ ...form, growth: v })}
          />
          <NumberField
            id="first_pct"
            label="初回の重さ（実力の%）"
            step={5}
            value={form.firstPct}
            fallback={defaults?.first_session_pct}
            onChange={(v) => setForm({ ...form, firstPct: v })}
          />
          <NumberField
            id="body_weight"
            label="体重（kg）"
            step={0.5}
            value={form.bodyWeight}
            fallback={defaults?.body_weight_kg}
            onChange={(v) => setForm({ ...form, bodyWeight: v })}
          />
        </div>

        <OneRepMaxGrid options={options} form={form} setForm={setForm} />

        {/* 自分の種目。本番で利用者が足す種目を、同じ3つ（名前・効き方・刻み）で
            書く。サーバーと同じ1行の書式のまま URL に載るので、Claude がクエリを
            書き換えて条件を変えられる。ID は並び順に u-sim01, u-sim02… で、
            1RM の上書きは orm に u-sim01:80 のように書く（応答の settings に出る）。 */}
        <label htmlFor="custom" className="grid gap-1.5">
          <span className="text-[12px] font-bold text-muted">
            自分の種目（名前|区分:寄与,区分:寄与|刻み を ; で並べる。寄与1.0の区分が1つ以上要る）
          </span>
          <textarea
            id="custom"
            rows={3}
            value={form.custom}
            placeholder="アイソラテラル・ロー|TRAP_MID:1,LAT:0.5,BICEPS:0.5,REAR_DELT:0.5|2.5;アイソラテラル・フロント・プルダウン|LAT:1,BICEPS:0.5|2.5"
            onChange={(ev) => setForm({ ...form, custom: ev.target.value })}
            className={`font-mono text-[12px] ${FIELD}`}
          />
        </label>
      </div>

      <div className="flex items-center gap-3">
        <button
          type="button"
          disabled={busy || form.declared.length === 0}
          onClick={run}
          className="rounded-md bg-amber px-4 py-1.5 text-[13px] font-bold text-ground disabled:opacity-40"
        >
          {busy ? '作成中' : '作る'}
        </button>
        <span className="text-[12px] text-faint">空欄はサーバーの既定値。変えた値だけ URL に載る</span>
      </div>
    </section>
  );
}

const FIELD = 'w-full rounded-md border border-line bg-ground px-2 py-1 text-[13px]';

/** Weekdays は通う曜日。未指定なら頻度ごとの既定の曜日を点けて出す。 */
function Weekdays({ options, form, setForm }: { options: DevOptions | null; form: Form; setForm: (f: Form) => void }) {
  const defaults = options?.schedule_defaults.weekdays_by_frequency ?? {};
  const on = form.days ?? defaults[String(form.frequency)] ?? [];
  const start = form.start ?? options?.schedule_defaults.start ?? '';
  return (
    <div className="flex gap-1">
      {[0, 1, 2, 3, 4, 5, 6].map((d) => (
        <button
          key={d}
          type="button"
          aria-pressed={on.includes(d)}
          onClick={() => setForm(toggleDay(form, d, defaults))}
          className={`h-7 w-7 rounded-md border text-[12px] ${
            on.includes(d) ? 'border-amber text-amber' : 'border-line text-muted'
          }`}
        >
          {weekdayName(start, d)}
        </button>
      ))}
    </div>
  );
}

/** NumberField は null（既定）を空欄で表す数値欄。既定値は placeholder に出す。 */
function NumberField({
  id,
  label,
  step,
  value,
  fallback,
  onChange,
}: {
  id: string;
  label: string;
  step: number;
  value: number | null;
  fallback: number | undefined;
  onChange: (v: number | null) => void;
}) {
  return (
    <label htmlFor={id} className="grid gap-1.5">
      <span className="text-[12px] font-bold text-muted">{label}</span>
      <input
        id={id}
        type="number"
        step={step}
        value={value ?? ''}
        placeholder={fallback === undefined ? '' : `既定 ${fallback}`}
        onChange={(ev) => onChange(ev.target.value === '' ? null : Number(ev.target.value))}
        className={`num ${FIELD}`}
      />
    </label>
  );
}

/** OneRepMaxGrid は全種目の初日の1RM。宣言種目を先頭に、折りたたまずに並べる。 */
function OneRepMaxGrid({
  options,
  form,
  setForm,
}: {
  options: DevOptions | null;
  form: Form;
  setForm: (f: Form) => void;
}) {
  const all = options?.exercises ?? [];
  const declared = form.declared.flatMap((id) => all.filter((e) => e.id === id));
  const rest = all.filter((e) => !form.declared.includes(e.id));

  const field = (e: DevExercise) => {
    const overridden = e.id in form.orm;
    return (
      <label key={e.id} className="flex items-center justify-between gap-2 text-[12px]">
        <span className={overridden ? 'text-amber' : 'text-muted'}>
          {e.name}
          {e.bodyweight && <span className="ml-1 text-faint">加重</span>}
        </span>
        <input
          type="number"
          step={2.5}
          aria-label={`${e.name}の1RM`}
          value={form.orm[e.id] ?? e.default_1rm_kg}
          onChange={(ev) =>
            setForm({ ...form, orm: setOneRepMax(form.orm, e.id, Number(ev.target.value), e.default_1rm_kg) })
          }
          className="num w-20 rounded-md border border-line bg-ground px-2 py-0.5 text-right text-[12px]"
        />
      </label>
    );
  };

  return (
    <div className="grid gap-2">
      <span className="text-[12px] font-bold text-muted">
        初日の実力（1RM, kg）。自重種目は加重の分だけ。既定値から変えた種目は黄色
      </span>
      <div className="grid grid-cols-4 gap-x-6 gap-y-1">{declared.map(field)}</div>
      <div className="grid grid-cols-4 gap-x-6 gap-y-1 border-t border-line-soft pt-2">{rest.map(field)}</div>
    </div>
  );
}

function Results({ options, result, ranForm }: { options: DevOptions | null; result: DevResult; ranForm: Form }) {
  // 自分の種目は options に無い（設定ごとに変わる）。結果の settings から足す。
  const names = Object.fromEntries([
    ...(options?.exercises ?? []).map((e) => [e.id, e.name]),
    ...(result.settings.custom ?? []).map((c) => [c.id, c.name]),
  ]);
  const splitName = options?.presets.find((p) => p.key === result.settings.split)?.name ?? result.settings.split;
  const series = weightSeries(
    result,
    result.settings.declared.map((id) => ({ id, name: names[id] ?? id })),
  );
  const from = result.days[0]?.date;
  const to = result.days[result.days.length - 1]?.date;

  return (
    <>
      <section className="grid gap-2 rounded-[14px] border border-line bg-surface p-4" aria-label="要約">
        <SectionTitle>要約</SectionTitle>
        <ul className="grid gap-1 text-[13px] leading-relaxed">
          <li>{settingsLine(result.settings, names, splitName)}</li>
          {series.map((s) => (
            <li key={s.id}>{liftLine(s)}</li>
          ))}
          {result.weeks.map((w) => (
            <li key={w.index} className="text-muted">
              {weekLine(w)}
            </li>
          ))}
        </ul>
        <div className="grid gap-1 border-t border-line-soft pt-2 text-[12px] text-muted">
          <div>
            この画面: <code className="num select-all text-text">{`${window.location.origin}${window.location.pathname}?${buildQuery(ranForm)}`}</code>
          </div>
          <div>
            JSON（トークンは $LIFTPLAN_TOKEN）: <code className="num select-all text-text">{curlCommand(API_BASE, ranForm)}</code>
          </div>
        </div>
      </section>

      {from && to && (
        <section className="grid gap-2">
          <SectionTitle>宣言種目の重量（記録した重さ・その日の実力）</SectionTitle>
          <WeightTrend series={series} from={from} to={to} />
        </section>
      )}

      <section className="grid gap-2">
        <SectionTitle>筋区分ごとの刺激（週目標に対する達成率）</SectionTitle>
        <RegionHeatmap weeks={result.weeks} />
      </section>

      <section className="grid gap-2">
        <SectionTitle>日ごとの計画と記録</SectionTitle>
        <div className="grid grid-cols-2 gap-2">
          {result.days.map((d) => (
            <DayCard key={d.date} day={d} />
          ))}
        </div>
      </section>
    </>
  );
}

function SectionTitle({ children }: { children: React.ReactNode }) {
  return <h2 className="text-[14px] font-bold">{children}</h2>;
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="grid gap-1.5">
      <div className="text-[12px] font-bold text-muted">{label}</div>
      {children}
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
            <span className="num text-muted">
              処方 {formatWeight(s)} ×{s.sets}セット RIR{s.target_rir}
            </span>
            <span className="num">→ 記録 {formatPerformed(s)}</span>
            {formatPct(s) && <span className="num text-[11px] text-amber">推定比 {formatPct(s)}</span>}
          </div>
        ))}
      </div>
    </div>
  );
}
