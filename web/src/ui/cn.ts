import { clsx, type ClassValue } from 'clsx';
import { extendTailwindMerge, validators } from 'tailwind-merge';

// cn はクラス名を合成する。shadcn/ui の作法に合わせてある。
//
// twMerge を通すのは、あとから渡したユーティリティが先のものに勝つようにするため。
// 素の clsx だけだと `px-4` と `px-2` が両方残り、どちらが効くかが
// CSS の並び順という見えないものに左右される。
//
// animate-* は index.css の @theme で足した名前なので、twMerge は知らない。
// 教えないと Card の既定の入りと、渡した入りが両方残る。
const twMerge = extendTailwindMerge({ extend: { theme: { animate: [validators.isAny] } } });

export const cn = (...inputs: ClassValue[]): string => twMerge(clsx(inputs));
