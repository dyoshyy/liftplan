type Props = {
  pending: number;
  rejected: number;
  online: boolean;
  onReload: () => void;
};

// StatusBar は「記録が送られたか」を常に見えるところに置く。
//
// ジムでは電波が切れる。送れていないことに気づけないのが一番まずい。
export function StatusBar({ pending, rejected, online, onReload }: Props) {
  const { color, text } = describe(pending, rejected, online);

  return (
    <div
      className="fixed inset-x-0 bottom-0 z-40 flex items-center gap-2.5 border-t border-line
                 bg-surface/95 px-4 text-[13px] backdrop-blur-[10px]"
      style={{ paddingBlock: '13px', paddingBottom: 'calc(13px + env(safe-area-inset-bottom))' }}
    >
      <span className={`size-2 flex-none rounded-full ${color}`} />
      <span>{text}</span>
      <button type="button" className="chip ml-auto px-[11px] py-[5px]" onClick={onReload}>
        更新
      </button>
    </div>
  );
}

function describe(pending: number, rejected: number, online: boolean) {
  // 送れなかった記録が一番強い。未送信より先に出す。
  if (rejected > 0) {
    return {
      color: 'bg-red',
      text:
        pending > 0
          ? `未送信 ${pending} 件・送れなかった記録 ${rejected} 件`
          : `送れなかった記録 ${rejected} 件`,
    };
  }
  if (!online) {
    return {
      color: 'bg-red',
      text: pending > 0 ? `オフライン・未送信 ${pending} 件` : 'オフライン（記録は保存されます）',
    };
  }
  return {
    color: pending > 0 ? 'bg-amber' : 'bg-green',
    text: pending > 0 ? `未送信 ${pending} 件` : '同期済み',
  };
}
