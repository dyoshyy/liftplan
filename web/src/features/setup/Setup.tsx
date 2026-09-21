import type { ReactElement, SVGProps } from 'react';
import { API_BASE } from '../../api/client';
import { Card, Note } from '../../ui/Card';
import { buttonStyles } from '../../ui/Button';
import { GitHubIcon, GoogleIcon } from '../../ui/icons';
import { cn } from '../../ui/cn';
import { loginUrl, type Provider } from './auth';

// ログイン画面。判断は auth.ts にあり、ここは描画だけをする。
//
// **素の <a> であることに意味がある。**fetch で叩くと、認可画面へのリダイレクトを
// 追いかけることになり、CORS で落ちるうえ、そもそも認可画面を人に見せられない。
// トップレベル遷移でプロバイダへ渡し、サーバーが #token= を付けて戻してくる。
//
// アイコンは描画の話なので、判断を置く auth.ts ではなくここに持つ。
const PROVIDERS: readonly {
  provider: Provider;
  label: string;
  Icon: (props: SVGProps<SVGSVGElement>) => ReactElement;
}[] = [
  { provider: 'github', label: 'GitHub でログイン', Icon: GitHubIcon },
  { provider: 'google', label: 'Google でログイン', Icon: GoogleIcon },
];

export function Setup({ pending }: { pending: number }) {
  return (
    <Card title="ログイン">
      <Note className="mb-3">
        記録はアカウントに紐づきます。この端末には、送るための印だけを保存します。
      </Note>
      <div className="grid gap-2">
        {PROVIDERS.map(({ provider, label, Icon }) => (
          <a
            key={provider}
            href={loginUrl(API_BASE, provider)}
            className={cn(buttonStyles({ variant: 'quiet' }), 'gap-2')}
          >
            <Icon />
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
