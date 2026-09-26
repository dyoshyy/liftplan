import { useEffect, useState } from 'react';
import { getJSON } from '../../api/client';
import type { AccountResponse, Login } from '../../api/types';

// ログイン方法を読む手順。表示の判断は account.ts にある。
//
// 読めなければ空のまま何も出さない。アドレスは確かめられれば便利という
// 程度のもので、読めないことを文言で知らせるほどではない。設定の保存の
// 失敗（useProgramSettings の note）と並べると、どちらが大事か分からなくなる。
export function useAccount(): readonly Login[] {
  const [logins, setLogins] = useState<readonly Login[]>([]);

  useEffect(() => {
    let alive = true;
    getJSON<AccountResponse>('/api/account')
      .then((r) => {
        if (alive) setLogins(r.accounts);
      })
      .catch(() => {});
    // 画面を閉じたあとに応答が来ても書き込まない。
    return () => {
      alive = false;
    };
  }, []);

  return logins;
}
