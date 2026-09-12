import { CheckCircle2 } from 'lucide-react';
import type { TableProgress } from './tableProgress';

export function TableStatusBadge({ progress }: { progress?: TableProgress }) {
  if (!progress || progress.prepared === 0) return null;
  const complete = progress.prepared === progress.total;
  return (
    <span
      title={`已保存 ${progress.prepared}/${progress.total} 轮制表内容；此标签不代表五维评分满分`}
      className={`inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[10px] font-semibold ${complete
        ? 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-500/20 dark:bg-emerald-500/10 dark:text-emerald-300'
        : 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-500/20 dark:bg-amber-500/10 dark:text-amber-300'}`}
    >
      {complete && <CheckCircle2 className="h-3 w-3" aria-hidden="true" />}
      {complete ? '已制表' : `制表 ${progress.prepared}/${progress.total}`}
    </span>
  );
}
