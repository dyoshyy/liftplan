import { cva, type VariantProps } from 'class-variance-authority';
import type { ButtonHTMLAttributes } from 'react';
import { cn } from './cn';

// ボタンの見た目はここ1箇所に集める。
//
// 以前は index.css の .btn / .btn-quiet / .btn-danger と、各画面での
// className の書き足しに散っていた。「主要な操作は黄、取り消しは赤」という
// 決まり（IPF のプレート色から来ている）が、どこを見れば分かるのかが無かった。
const button = cva(
  'inline-flex items-center justify-center rounded-xl text-center transition-transform ' +
    'active:scale-[.985] disabled:pointer-events-none disabled:opacity-50 ' +
    'focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-amber',
  {
    variants: {
      variant: {
        // 15kg プレートの黄。その画面で一番やりたいこと。
        primary: 'bg-amber font-bold text-[#1a1204]',
        quiet: 'border border-line bg-surface-2 font-medium text-text',
        // 25kg プレートの赤。取り消し。
        danger: 'border border-red/55 bg-transparent font-medium text-red',
        ghost: 'text-muted hover:text-text',
      },
      size: {
        block: 'w-full px-4 py-[15px]',
        md: 'px-4 py-2.5 text-sm',
        chip: 'rounded-full px-3 py-2 text-[13px]',
        icon: 'size-11 rounded-xl text-2xl leading-none',
      },
    },
    defaultVariants: { variant: 'primary', size: 'block' },
  },
);

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> &
  VariantProps<typeof button>;

export function Button({ className, variant, size, type = 'button', ...props }: ButtonProps) {
  return <button type={type} className={cn(button({ variant, size }), className)} {...props} />;
}
