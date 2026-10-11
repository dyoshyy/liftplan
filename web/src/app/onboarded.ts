import type { OnboardingResponse } from '../api/types';

/**
 * onboardedFrom は初回の案内を済ませたかを決める。
 *
 * 取れなかった（null）ときは済んだものとして扱う。圏外で開いた日に
 * 設定画面で止めると、記録すらできなくなる。案内は次に取れたときに出る。
 */
export function onboardedFrom(res: OnboardingResponse | null): boolean {
  return res?.completed ?? true;
}
