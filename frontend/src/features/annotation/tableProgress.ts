import type { AnnotationCase, AnnotationRound } from '../../api/annotation';

export type TableProgress = {
  prepared: number; total: number; reviewed?: number; missing?: number; stale?: number;
  state?: 'pending' | 'running' | 'error' | 'cancelled' | 'ready' | 'partial' | 'needs_evidence' | 'stale' | 'captured' | 'empty';
  message?: string; error?: string; quiet?: boolean; elapsed?: number; lastActivityAt?: number;
};

export function latestEvaluation(round: AnnotationRound) {
  return [...(round.evaluations ?? [])].sort((a,b) => (a.createdAt ?? 0)-(b.createdAt ?? 0)).at(-1);
}

// Match the saved evaluations accepted by the reviewed-only export. A pending
// round still belongs in the total so adding a round removes the complete badge.
export function getTableProgress(item: Pick<AnnotationCase, 'rounds' | 'preparation'>, now = Date.now()/1000): TableProgress {
  const rounds = item.rounds.filter((round) => round.status !== 'excluded');
  let prepared=0, reviewed=0, missing=0, stale=0;
  for (const round of rounds) {
    const e=latestEvaluation(round);
    if (!e) continue;
    reviewed++;
    if (e.current === false || e.evidenceHash !== round.evidenceHash) { stale++; continue; }
    if (e.status === 'needs_evidence') missing++;
    if (round.status === 'complete' && e.status === 'ready') prepared++;
  }
  const p=item.preparation;
  const active=p?.status === 'pending' || p?.status === 'running';
  const failed=p?.status === 'error' || p?.status === 'cancelled';
  const state: TableProgress['state'] = active || failed ? p!.status as TableProgress['state']
    : stale ? 'stale' : missing ? 'needs_evidence' : prepared && prepared===rounds.length ? 'ready'
    : prepared ? 'partial' : rounds.length ? 'captured' : 'empty';
  return {prepared,total:rounds.length,reviewed,missing,stale,state,message:p?.message,error:p?.error,
    quiet: p?.status === 'running' && now-p.lastActivityAt>=120,
    elapsed: p ? Math.max(0,Math.floor((p.finishedAt || now)-p.startedAt)) : 0,lastActivityAt:p?.lastActivityAt};
}
