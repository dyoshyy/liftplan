import type { Program, RepTargets } from '../../api/types';

/** REP_OPTIONS は選べるレップ数。サーバーの範囲（1〜15）と同じ。 */
export const REP_OPTIONS: number[] = Array.from({ length: 15 }, (_, i) => i + 1);

/** repsPath は1つの宣言のレップ数を差し替える口。 */
export const repsPath = (id: string): string => `/api/program/declared/${encodeURIComponent(id)}/reps`;

/** repsRows は宣言ごとのレップ数を、宣言の順に並べる。
 *
 *  値が無い宣言は出さない。宣言を足した直後は取り直すまで値が無いが、
 *  既定（3・6）を手元で作ると、サーバーの既定と二重に持つことになる。 */
export const repsRows = (program: Program): { id: string; reps: RepTargets }[] =>
  program.declared_exercises.flatMap((id) => {
    const reps = program.declared_reps[id];
    return reps ? [{ id, reps }] : [];
  });
