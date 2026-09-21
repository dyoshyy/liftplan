import { API_BASE } from '../../api/client';
import { Card, Note } from '../../ui/Card';
import { buttonStyles } from '../../ui/Button';
import { loginUrl, type Provider } from './auth';

// ログイン画面。判断は auth.ts にあり、ここは描画だけをする。
//
// **素の <a> であることに意味がある。**fetch で叩くと、認可画面へのリダイレクトを
// 追いかけることになり、CORS で落ちるうえ、そもそも認可画面を人に見せられない。
// トップレベル遷移でプロバイダへ渡し、サーバーが #token= を付けて戻してくる。
const PROVIDERS: readonly { provider: Provider; label: string }[] = [
  { provider: 'github', label: 'GitHub でログイン' },
  { provider: 'google', label: 'Google でログイン' },
];

export function Setup({ pending }: { pending: number }) {
  return (
    <Card title="ログイン">
      <Note className="mb-3">
        記録はアカウントに紐づきます。この端末には、送るための印だけを保存します。
      </Note>
      <div className="grid gap-2">
        {PROVIDERS.map(({ provider, label }) => (
          <a key={provider} href={loginUrl(API_BASE, provider)} className={buttonStyles({ variant: 'quiet' })}>
            {label}
          </a>
        ))}
      </div>
      {/* 溜まっているものは消えない。ここで言わないと、記録ごと消えたと
          思われる。ログインし直すのは送り先の話で、記録の話ではない。 */}
      {pending > 0 && (
        <Note className="mt-3">未送信の記録が {pending} 件あります。ログインすれば送られます。</Note>
      )}
    </Card>
  );
}
