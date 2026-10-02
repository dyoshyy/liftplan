import { add, addAll, clear, count, head, remove, values, type QueueItem } from './db';

/** Send は1件を実際に投げる。fetch そのものではなく関数で受け取る。
 *
 * こうしてあるのは、再送の規則をネットワーク無しで検査できるようにするため。
 * Outbox は fetch も React も知らない。 */
export type Send = (item: QueueItem) => Promise<Response>;

/** describe は捨てたものを人が読める1行にする。
 *
 * path と JSON をそのまま出しても、何を失ったのか分からない。 */
function describe(item: QueueItem, status: number, nameOf: (id: string) => string): string {
  const body = item.body as
    | { logs?: { date: string; exercise_id: string; weight_kg: number; reps: number }[] }
    | { conditions?: { date: string }[] }
    | undefined;

  const log = (body as { logs?: { date: string; exercise_id: string; weight_kg: number; reps: number }[] })
    ?.logs?.[0];
  if (log) {
    return `${log.date} ${nameOf(log.exercise_id)} ${log.weight_kg}kg × ${log.reps}（${status}）`;
  }
  const condition = (body as { conditions?: { date: string }[] })?.conditions?.[0];
  if (condition) return `${condition.date} のコンディション（${status}）`;
  return `${item.method ?? 'POST'} ${item.path}（${status}）`;
}

// Outbox は記録を失わないための待ち行列。
//
// 記録は待ち行列に積んでから送る。ジムの電波は途切れる。保存してから
// 送れば、失敗しても消えない。ID はクライアントが採番するので、
// 再送は安全（サーバーが冪等）。
export class Outbox {
  private flushing = false;
  /** nameOf は捨てた記録の説明に使う種目名。読み込み後に差し替わる。 */
  nameOf: (id: string) => string = (id) => id;

  constructor(private readonly send: Send) {}

  pendingCount(): Promise<number> {
    return count('queue');
  }

  rejected(): Promise<string[]> {
    return values<string>('rejected');
  }

  clearRejected(): Promise<void> {
    return clear('rejected');
  }

  enqueue(item: QueueItem): Promise<void> {
    return add('queue', item);
  }

  // enqueueAll は複数件を1回の書き込みで積む。全部積めるか、1件も積まれないか。
  // 修正（DELETE と POST の2件）は必ずこれで積む。
  enqueueAll(items: readonly QueueItem[]): Promise<void> {
    return addAll('queue', items);
  }

  // flush は先頭から順に送る。
  //
  // 先頭で止めるのは、順序が意味を持つため。記録の修正は
  // 「DELETE を積んでから同一IDで POST」の2件で表され、
  // 入れ替わると消したはずの記録が残る。
  async flush(): Promise<void> {
    // 同時に2回走ると、同じ先頭を2回送って二重に記録される。
    if (this.flushing) return;
    this.flushing = true;
    try {
      for (;;) {
        const entry = await head<QueueItem>('queue');
        if (!entry) break;

        const res = await this.send(entry.value);

        if (!res.ok && res.status >= 400 && res.status < 500) {
          // 再送しても永久に通らない。残すと後続が全部詰まる。
          // ただし黙って消すと、記録したはずのものが無いことに気づけない。
          await add('rejected', describe(entry.value, res.status, this.nameOf));
        } else if (!res.ok) {
          break; // 5xx / 503 はやり直せば通る
        }
        await remove('queue', entry.key);
      }
    } catch {
      // 電波が無い。次の機会に送る。待ち行列は減らさない。
    } finally {
      this.flushing = false;
    }
  }
}
