import type { ForecastSession, PlannedSet } from '../../api/types';
import { BackButton } from '../../ui/BackButton';
import { Card, Note } from '../../ui/Card';
import { forecastRows, sessionHeading } from './forecast';
import { useForecast } from './useForecast';

type Props = {
  nameOf: (id: string) => string;
  onBack: () => void;
};

// Forecast はこの先の回を全部開いて並べる。
//
// 以前は今日だけ開き、先の回はたたんでいた。この画面に来る目的は
// 「次に何をやるか」を見ることで、たたまれていると1回ずつ開く手間だけが
// 残る。1回ぶんは数行なので、全部開いても1画面に2〜3回は収まる。
export function Forecast({ nameOf, onBack }: Props) {
  const { sessions, error } = useForecast();

  return (
    <div className="grid gap-3.5">
      <BackButton label="今日" onClick={onBack} />

      <div>
        <h1 className="text-[19px] font-semibold">この先の予定</h1>
        <Note className="mt-1">
          今日の時点の見込みです。休んだり、別の種目をやったりすると変わります。
          重量は今日の実力での見込みで、当日の記録に合わせて付け直されます。
        </Note>
      </div>

      {error && <Note>{error}</Note>}

      {sessions?.map((session) => (
        <SessionCard key={session.index} session={session} nameOf={nameOf} />
      ))}
    </div>
  );
}

function SessionCard({ session, nameOf }: { session: ForecastSession; nameOf: (id: string) => string }) {
  return (
    <Card title={sessionHeading(session.index, session.split)}>
      <div className="grid gap-2.5">
        {forecastRows(session).map((set) => (
          <Row key={set.exercise_id} set={set} name={nameOf(set.exercise_id)} />
        ))}
      </div>
    </Card>
  );
}

// Row は1種目を1行で出す。今日の画面の Target（重量 34px）は使わない。
// 回が6つ並ぶ画面で1種目ごとに大きな数字を出すと、縦に長くなるだけで
// 見比べられない。
function Row({ set, name }: { set: PlannedSet; name: string }) {
  return (
    <div className="grid grid-cols-[1fr_auto] items-baseline gap-3">
      <span className="text-[15px]">{name}</span>
      <span className="num text-right text-[13px] text-muted">
        {set.weight_kg === null ? (
          '自分で決める'
        ) : (
          <span className="text-[17px] font-medium text-text">
            {set.weight_kg}
            <small className="text-xs font-normal text-muted">kg</small>
          </span>
        )}
        <span className="ml-2">
          {set.sets}×{set.target_reps}
        </span>
      </span>
    </div>
  );
}
