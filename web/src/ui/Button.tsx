import { cva, type VariantProps } from 'class-variance-authority';
import type { ButtonHTMLAttributes } from 'react';
import { cn } from './cn';

// ボタンの見た目はここ1箇所に集める。
//
// 以前は index.css の .btn / .btn-quiet / .btn-danger と、各画面での
// className の書き足しに散っていた。「主要な操作は黄、取り消しは赤」という
// 決まり（IPF のプレート色から来ている）が、どこを見れば分かるのかが無かった。
// buttonStyles は <a> にも同じ見た目を当てるために公開する。
// ログインはトップレベル遷移なので、button ではなく a でなければならない。
export const buttonStyles = cva(
  'inline-flex items-center justify-center rounded-xl text-center transition-transform ' +
    'active:scale-[.985] disabled:pointer-events-none disabled:opacity-50 ' +
    'focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-amber',
  {
    variants: {
      variant: {
        // 15kg プレートの黄。その画面で一番やりたいこと。
        primary: 'bg-amber font-bold text-[#1a1204]',
        quiet: 'border border-line bg-surface-2 font-medium text-text',
        // 複数選べる一覧の「選択中」。
        //
        // primary（ベタ塗りの黄）を使わない。黄は「この画面で一番やりたい
        // こと」の色で、30個に付くと意味を失う。縁と文字だけ黄にして、
        // 選ばれていることは分かるが主張はしない状態にする。
        selected: 'border border-amber/55 bg-amber/15 font-medium text-amber',
        // 25kg プレートの赤。取り消し。
        danger: 'border border-red/55 bg-transparent font-medium text-red',
        ghost: 'text-muted hover:text-text',
      },
      size: {
        block: 'w-full px-4 py-[15px]',
        md: 'min-h-11 px-4 py-2.5 text-sm',
        // 高さ 44px は指で押す的の下限。以前は 39px で、設定画面の
        // 77個すべてが基準未満だった。
        chip: 'min-h-11 rounded-full px-3.5 py-2 text-[13px]',
        icon: 'size-11 rounded-xl text-2xl leading-none',
      },
    },
    defaultVariants: { variant: 'primary', size: 'block' },
  },
);

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & VariantProps<typeof buttonStyles>;

export function Button({ className, variant, size, type = 'button', ...props }: ButtonProps) {
  return <button type={type} className={cn(buttonStyles({ variant, size }), className)} {...props} />;
}
