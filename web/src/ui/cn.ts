import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

// cn はクラス名を合成する。shadcn/ui の作法に合わせてある。
//
// twMerge を通すのは、あとから渡したユーティリティが先のものに勝つようにするため。
// 素の clsx だけだと `px-4` と `px-2` が両方残り、どちらが効くかが
// CSS の並び順という見えないものに左右される。
export const cn = (...inputs: ClassValue[]): string => twMerge(clsx(inputs));
