import { useState } from 'react';
import type { ForecastSession, PlannedSet } from '../../api/types';
import { Card, Note } from '../../ui/Card';
import { Target } from '../today/ExerciseCard';
import { forecastRows, isInitiallyOpen, sessionHeading } from './forecast';
import { useForecast } from './useForecast';

type Props = {
  nameOf: (id: string) => string;
  onBack: () => void;
};

export function Forecast({ nameOf, onBack }: Props) {
  const { sessions, error } = useForecast();
  // どの回を開いて始めるかは forecast.ts の isInitiallyOpen が決める
  // （今日＝回0だけ）。一度開閉を操作したあとは、押した回だけを覚える
  // 単純な状態にする。
  const [openIndex, setOpenIndex] = useState<number>(() =>
    isInitiallyOpen(0) ? 0 : -1,
  );

  return (
    <div className="grid gap-3.5">
      <button type="button" onClick={onBack} className="w-fit text-sm text-muted">
        ← 今日
      </button>

      <div>
        <h1 className="text-[19px] font-semibold">この先の予定</h1>
        <Note className="mt-1">
          今日の時点の見込みです。休んだり、別の種目をやったりすると変わります。
          重量は今日の実力での見込みで、当日の記録に合わせて付け直されます。
        </Note>
      </div>

      {error && <Note>{error}</Note>}

      {sessions?.map((session) => (
        <SessionCard
          key={session.index}
          session={session}
          nameOf={nameOf}
          open={openIndex === session.index}
          onToggle={() =>
            setOpenIndex((cur) => (cur === session.index ? -1 : session.index))
          }
        />
      ))}
    </div>
  );
}

function SessionCard({
  session,
  nameOf,
  open,
  onToggle,
}: {
  session: ForecastSession;
  nameOf: (id: string) => string;
  open: boolean;
  onToggle: () => void;
}) {
  const rows = forecastRows(session);
  return (
    <Card className="grid gap-2.5">
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={open}
        className="text-left text-[15px] font-semibold"
      >
        {open ? '▼' : '▶'} {sessionHeading(session.index, session.split)}
      </button>
      {open && (
        <div className="grid gap-3">
          {rows.map((set) => (
            <Row key={set.exercise_id} set={set} name={nameOf(set.exercise_id)} />
          ))}
        </div>
      )}
    </Card>
  );
}

function Row({ set, name }: { set: PlannedSet; name: string }) {
  return (
    <div className="grid gap-1">
      <span className="text-[15px] font-medium">{name}</span>
      <Target plan={set} />
    </div>
  );
}
