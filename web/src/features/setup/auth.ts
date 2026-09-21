// ログインの判断。純粋関数だけを置く。
//
// ログイン画面（Setup.tsx）と、戻ってきたときの取り込み
// （useSessionOrchestrator）の両方から使うので、最初から別ファイルにする。

/** Provider は使えるログイン手段。サーバーの /auth/{provider}/start と対。 */
export type Provider = 'github' | 'google';

/** loginUrl は認可を始める URL。**fetch ではなくトップレベル遷移で開くこと。**
 *  fetch にすると CORS とリダイレクト追従の問題になり、そもそも認可画面を
 *  人に見せられない。 */
export const loginUrl = (apiBase: string, provider: Provider): string =>
  `${apiBase}/auth/${provider}/start`;

/** tokenFromHash はコールバックが置いていったトークンを取り出す。無ければ null。
 *
 *  クエリではなくフラグメントなのはサーバー側の判断で、Referer と
 *  アクセスログにトークンを残さないため。 */
export function tokenFromHash(hash: string): string | null {
  // URLSearchParams を通すのは、パーセントエンコードを戻すため。
  // 素の split で切ると、/ や + を含むトークンが別物のまま保存され、
  // 通らないトークンで 401 になる。
  const token = new URLSearchParams(hash.replace(/^#/, '')).get('token')?.trim() ?? '';
  return token === '' ? null : token;
}

/** IntakeStep は取り込みで何をするか。実行はしない。 */
export type IntakeStep =
  | { kind: 'saveToken'; token: string }
  | { kind: 'clearHash' }
  | { kind: 'signIn' };

/** IntakePorts は IntakeStep の実体。副作用はすべてここから外へ出す。 */
export type IntakePorts = {
  saveToken: (token: string) => void;
  clearHash: () => void;
  signIn: () => void;
};

/** planTokenIntake は戻ってきたときに何をどの順ですべきかを決める。
 *
 *  順序に意味がある。保存より先にログイン済みにすると、読み込みが走った
 *  ときにトークンがまだ無く、そのまま 401 で弾き返される。URL から消すのを
 *  最後にすると、その間の再読み込みで同じフラグメントをもう一度取り込み、
 *  401 で捨てたはずの古いトークンが URL から蘇る。 */
export function planTokenIntake(hash: string): IntakeStep[] {
  const token = tokenFromHash(hash);
  if (token === null) return [];
  return [{ kind: 'saveToken', token }, { kind: 'clearHash' }, { kind: 'signIn' }];
}

/** applyTokenIntake は決まった順にそのまま実行する。並べ替えない。 */
export function applyTokenIntake(steps: readonly IntakeStep[], ports: IntakePorts): void {
  for (const step of steps) {
    switch (step.kind) {
      case 'saveToken':
        ports.saveToken(step.token);
        break;
      case 'clearHash':
        ports.clearHash();
        break;
      case 'signIn':
        ports.signIn();
        break;
    }
  }
}
