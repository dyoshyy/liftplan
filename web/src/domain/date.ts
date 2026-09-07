// today は端末の日付を YYYY-MM-DD で返す。
//
// toISOString を使わない。あれは UTC に直すので、日本時間の朝9時より前は
// 前日になる。ジムの朝練の記録が前日に付く。
export function today(now: Date = new Date()): string {
  const p = (n: number) => String(n).padStart(2, '0');
  return `${now.getFullYear()}-${p(now.getMonth() + 1)}-${p(now.getDate())}`;
}

// addDays は YYYY-MM-DD に日数を足す。
export function addDays(iso: string, days: number): string {
  const d = new Date(`${iso}T00:00:00`);
  d.setDate(d.getDate() + days);
  return today(d);
}

// label は「9/7 (日)」の形にする。
export function label(iso: string): string {
  const d = new Date(`${iso}T00:00:00`);
  const weekday = '日月火水木金土'[d.getDay()] ?? '';
  return `${d.getMonth() + 1}/${d.getDate()} (${weekday})`;
}
