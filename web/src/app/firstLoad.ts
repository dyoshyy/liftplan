import type { Session } from '../api/types';
import type { LoadState } from './useSessionOrchestrator';

/** awaitingFirstLoad は、まだ一度も読めておらず、いま読んでいる最中か。
 *  スケルトンを出すのはこのときだけ。 */
export const awaitingFirstLoad = (load: LoadState, session: Session | null): boolean =>
  load === 'loading' && session === null;
