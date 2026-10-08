import { ChevronLeftIcon } from './icons';

// BackButton は子画面の頭に置く「1つ上へ」。
//
// 以前は 14px の文字だけで、押す的が小さく見落とされた。高さ 44px は
// 指で押す的の下限（Button の size と同じ基準）。
export function BackButton({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="-ml-2 inline-flex min-h-11 w-fit items-center gap-0.5 rounded-xl pl-1 pr-3 text-[15px] text-muted active:text-text"
    >
      <ChevronLeftIcon width={22} height={22} />
      {label}
    </button>
  );
}
