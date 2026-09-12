import type { AnnotationCase } from '../../api/annotation';

export type TableProgress = { prepared: number; total: number };

// Match the saved evaluations accepted by the reviewed-only export. A pending
// round still belongs in the total so adding a round removes the complete badge.
export function getTableProgress(item: Pick<AnnotationCase, 'rounds'>): TableProgress {
  const rounds = item.rounds.filter((round) => round.status !== 'excluded');
  return {
    total: rounds.length,
    prepared: rounds.filter((round) => round.status === 'complete' &&
      (round.evaluations ?? []).some((evaluation) => evaluation.evidenceHash === round.evidenceHash &&
        (evaluation.status === 'ready' || evaluation.status === 'needs_evidence'))).length,
  };
}
