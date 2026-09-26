import { useCallback, useState } from 'react';
import { send } from '../../api/client';
import { clearToken } from '../../storage/local';

/** LogoutPorts はログアウトの副作用の実体。すべてここから外へ出す。 */
export type LogoutPorts = {
  /** revoke はサーバー側のセッションを失効させる。結果は見ない。 */
  revoke: () => Promise<unknown>;
  clearToken: () => void;
  forget: () => void;
};

/** runLogout はログアウトを決まった順に実行する。
 *
 *  **順序に意味がある。**失効の要求にはトークンを付ける。先に端末から
 *  捨てると付けるものが無くなり、漏れたトークンが期限（90日）まで使える。
 *
 *  失効の成否は見ない。圏外・5xx・401 のどれでも端末のトークンは捨てて
 *  先へ進む。失敗を理由に残すと、圏外ではログアウトできなくなる。
 *  共有の端末から立ち去れないほうが、サーバーに行が残るより困る。
 *  やり直しもしない。トークンを捨てたあとでは、もう本人として要求できない。 */
export async function runLogout(ports: LogoutPorts): Promise<void> {
  try {
    await ports.revoke();
  } catch {
    // 上のとおり。ここで止めない。
  }
  ports.clearToken();
  ports.forget();
}

// ログアウトの手順。判断は runLogout にあり、ここは実体を繋ぐだけ。
//
// 失効は api/client の send で送る。send は 401 を受けると自分で
// clearToken して Unauthorized を投げるが、どのみち直後に捨てるもの
// なので結果は同じになる。ここのために別の送り方を作らない。
export function useLogout(onForget: () => void) {
  // 応答を待つ間にもう一度押せると、失効の要求が二重に出る。
  // 戻さないのは、終わったときには画面ごとログインへ切り替わっているため。
  const [busy, setBusy] = useState(false);

  const logout = useCallback(async () => {
    setBusy(true);
    await runLogout({
      revoke: () => send({ path: '/auth/session', method: 'DELETE' }),
      clearToken,
      forget: onForget,
    });
  }, [onForget]);

  return { logout, busy };
}
