import { regionLabel } from '../domain/regions';
import { heatRows, type HeatCell, type HeatLevel } from './chart';
import { outOfRange, type DevWeek } from './simulate';

// 筋区分ごとの刺激。区分を行、週を列にして、目標に対する達成率を塗る。
//
// 発散配色。目標どおり（100%付近）が灰色で、不足が橙、過剰が青。目標を
// 中心に、どちらへどれだけ外れたかを見る。段階は3つずつ。
//
// 色だけに頼らない。セルに数字を出し、許容帯（60〜145%）の外には枠を付ける。
// 数字が読めれば、この表がそのまま表の表示になる。
//
// 値は暗い面（surface #161c22）の上で決めた。橙・青の極は categorical の
// 2番と1番の暗色（#d95926 / #3987e5）、中点は灰（#383835）で、間を
// sRGB で3等分している。文字は段階ごとに 4.5:1 を満たす側を選んだ。

const HEAT: Record<HeatLevel, { fill: string; ink: string }> = {
  [-3]: { fill: '#d95926', ink: '#0e1216' },
  [-2]: { fill: '#a34e2b', ink: '#e7edf3' },
  [-1]: { fill: '#6e4330', ink: '#e7edf3' },
  [0]: { fill: '#383835', ink: '#e7edf3' },
  [1]: { fill: '#385270', ink: '#e7edf3' },
  [2]: { fill: '#396daa', ink: '#e7edf3' },
  [3]: { fill: '#3987e5', ink: '#0e1216' },
};

// 左の2列は横スクロールしても残す。週が12あると、区分名が見えないまま
// 数字だけを読むことになる。背景を塗るのは、下を流れるセルを隠すため。
// 部位の列は折り返さない（「背中」が縦に割れる）。
//
// 外側に余白（p-2）を付けない。固定した列は、余白の内側ではなく枠の端に
// 貼り付くので、余白があると、その幅ぶんスクロールしたセルが左に漏れる。
// 余白は左端の列の中（pl-2）に持たせる。
//
// 2つの固定列の隙間（border-spacing）にもセルが漏れる。区分の列は
// 部位の列の右端（56px）に貼る。幅は px で揃える。em だと、部位の列
// （12px）と区分の列（11px）で違う長さになり、重なる。
const PART_COL = 'sticky left-0 z-10 w-[56px] min-w-[56px] whitespace-nowrap bg-surface pl-2';
const LABEL_COL = 'sticky left-[56px] z-10 min-w-[128px] whitespace-nowrap bg-surface';

const LEVELS: HeatLevel[] = [-3, -2, -1, 0, 1, 2, 3];
const OUT_RING = 'inset 0 0 0 2px #e7edf3';

export function RegionHeatmap({ weeks }: { weeks: DevWeek[] }) {
  const rows = heatRows(weeks);

  return (
    <div className="grid gap-2">
      <Legend />

      <div className="overflow-x-auto rounded-[14px] border border-line bg-surface py-2 pr-2">
        <table className="w-full border-separate border-spacing-[2px] text-[11px]">
          <thead>
            <tr className="text-muted">
              <th scope="col" className={`${PART_COL} text-left font-normal`}>
                部位
              </th>
              <th scope="col" className={`${LABEL_COL} text-left font-normal`}>
                筋区分
              </th>
              {weeks.map((w) => (
                <th key={w.index} scope="col" className="num min-w-[3.4em] font-normal">
                  {w.index}週
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row, i) => {
              const startsPart = i === 0 || rows[i - 1]!.part !== row.part;
              const span = rows.filter((r) => r.part === row.part).length;
              return (
                <tr key={row.region}>
                  {startsPart && (
                    <th
                      scope="rowgroup"
                      rowSpan={span}
                      className={`${PART_COL} text-left align-top text-[12px] font-bold text-muted`}
                    >
                      {row.part}
                    </th>
                  )}
                  <th scope="row" className={`${LABEL_COL} text-left font-normal`}>
                    {regionLabel(row.region)}
                  </th>
                  {row.cells.map((c) => (
                    <Cell key={c.week} cell={c} region={regionLabel(row.region)} />
                  ))}
                </tr>
              );
            })}
          </tbody>
          <tfoot>
            <tr className="text-muted">
              {/* 2列にまたがせない。またがると、幅の配分で部位の列が広がり、
                  固定した2列の位置がずれる。 */}
              <td className={PART_COL} />
              <th scope="row" className={`${LABEL_COL} text-left font-normal`}>
                許容外の区分
              </th>
              {weeks.map((w) => (
                <td key={w.index} className="num text-center">
                  {outOfRange(w).length}
                </td>
              ))}
            </tr>
          </tfoot>
        </table>
      </div>
    </div>
  );
}

function Cell({ cell, region }: { cell: HeatCell; region: string }) {
  if (cell.rate === null || cell.level === null) {
    return (
      <td className="text-center text-faint" title={`${region} ${cell.week}週目: 目標なし`}>
        –
      </td>
    );
  }

  const { fill, ink } = HEAT[cell.level];
  const out = Math.abs(cell.level) === 3;
  const pct = Math.round(cell.rate * 100);
  return (
    <td
      className="num rounded-[4px] py-1 text-center"
      style={{ background: fill, color: ink, boxShadow: out ? OUT_RING : undefined }}
      title={`${region} ${cell.week}週目: 目標 ${cell.target} / 実測 ${cell.done.toFixed(1)}（${pct}%）${
        out ? ' 許容外' : ''
      }`}
    >
      {pct}%
    </td>
  );
}

function Legend() {
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-[12px] text-muted">
      <span className="inline-flex items-center gap-1.5">
        不足
        <span className="inline-flex gap-[2px]" aria-hidden="true">
          {LEVELS.map((l) => (
            <span
              key={l}
              className="h-3 w-5 rounded-[3px]"
              style={{ background: HEAT[l].fill, boxShadow: Math.abs(l) === 3 ? OUT_RING : undefined }}
            />
          ))}
        </span>
        過剰
      </span>
      <span>灰 = 目標どおり（95〜105%）</span>
      <span>白枠 = 許容外（60% 未満 / 145% 超）</span>
    </div>
  );
}
