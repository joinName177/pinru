import { CheckCircle2, FileSearch, GitBranch, Loader2, Save, Sparkles } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import {
  capturePairwiseSide,
  commitPairwiseSide,
  preparePairwiseSide,
  reviewPairwise,
  savePairwiseMaterials,
  type AnnotationCase,
  type PairwiseRun,
  type PairwiseSide,
} from '../../api/annotation';
import type { BackgroundJob } from '../../api/job';

const INPUT = 'w-full rounded-lg border border-stone-200 bg-white px-3 py-2 text-sm text-stone-800 outline-none focus:border-slate-400 focus:ring-2 focus:ring-slate-200 dark:border-stone-700 dark:bg-[#171B22] dark:text-stone-100';
const PRIMARY = 'inline-flex h-9 items-center justify-center gap-2 rounded-lg bg-slate-800 px-3 text-sm font-semibold text-white hover:bg-slate-700 disabled:cursor-not-allowed disabled:opacity-40 dark:bg-slate-100 dark:text-slate-900';
const SECONDARY = 'inline-flex h-9 items-center justify-center gap-2 rounded-lg border border-stone-200 bg-white px-3 text-sm font-semibold text-stone-700 hover:bg-stone-50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-stone-700 dark:bg-stone-800 dark:text-stone-200';

export type PairwiseRunJob = (label: string, submit: () => Promise<BackgroundJob>) => Promise<boolean>;

type Props = {
  annotationCase: AnnotationCase;
  disabled: boolean;
  runJob: PairwiseRunJob;
};

function shortSha(value: string) {
  return value ? value.slice(0, 10) : '尚未提交';
}

function RunPanel({ taskId, side, run, disabled, runJob }: {
  taskId: string;
  side: PairwiseSide;
  run: PairwiseRun;
  disabled: boolean;
  runJob: PairwiseRunJob;
}) {
  const [tracePath, setTracePath] = useState(run.tracePath || '');
  const [videoUrl, setVideoUrl] = useState(run.videoUrl || '');
  useEffect(() => setTracePath(run.tracePath || ''), [run.tracePath]);
  useEffect(() => setVideoUrl(run.videoUrl || ''), [run.videoUrl]);
  const readyVideo = run.videoStatus === 'ready';

  return (
    <section role="region" aria-label={`运行 ${side}`} className="border border-stone-200 bg-white p-4 dark:border-stone-700 dark:bg-stone-900">
      <div className="flex items-center justify-between gap-3 border-b border-stone-100 pb-3 dark:border-stone-800">
        <div>
          <h3 className="text-base font-bold text-stone-900 dark:text-stone-100">运行 {side}</h3>
          <p className="mt-1 text-xs text-stone-500">固定分支 {side} · {run.preparedAt ? '已从初始快照准备' : '等待准备'}</p>
        </div>
        <button className={SECONDARY} disabled={disabled} onClick={() => void runJob(`准备 ${side} 分支`, () => preparePairwiseSide({ taskId, side }))}>
          <GitBranch className="h-4 w-4" />准备 {side}
        </button>
      </div>

      <dl className="mt-3 grid grid-cols-2 gap-3 text-xs">
        <div><dt className="text-stone-400">SessionID</dt><dd className="mt-1 break-all font-mono text-stone-700 dark:text-stone-300">{run.sessionId || '尚未采集'}</dd></div>
        <div><dt className="text-stone-400">产物 commit</dt><dd className="mt-1 font-mono text-stone-700 dark:text-stone-300">{shortSha(run.deliverableSha)}</dd></div>
      </dl>

      <div className="mt-4 space-y-3">
        <label className="block">
          <span className="mb-1 block text-xs font-semibold text-stone-500">首轮轨迹 JSONL</span>
          <input aria-label={`${side} 轨迹路径`} className={INPUT} value={tracePath} onChange={(event) => setTracePath(event.target.value)} placeholder="/absolute/path/to/session.jsonl" />
        </label>
        <div className="flex flex-wrap gap-2">
          <button className={PRIMARY} disabled={disabled || !tracePath.trim()} onClick={() => void runJob(`采集 ${side}`, () => capturePairwiseSide({ taskId, side, tracePath: tracePath.trim() }))}>
            <FileSearch className="h-4 w-4" />采集 {side}
          </button>
          <button className={SECONDARY} disabled={disabled || !run.sessionId} onClick={() => void runJob(`提交 ${side} 产物`, () => commitPairwiseSide({ taskId, side, sessionId: run.sessionId }))}>
            <GitBranch className="h-4 w-4" />提交 {side} 产物
          </button>
        </div>

        <label className="block">
          <span className="mb-1 block text-xs font-semibold text-stone-500">运行视频链接</span>
          <input aria-label={`${side} 视频链接`} className={INPUT} value={videoUrl} onChange={(event) => setVideoUrl(event.target.value)} placeholder="https://.../recording.mp4" />
        </label>
        <div className="flex flex-wrap items-center gap-2">
          <button className={SECONDARY} disabled={disabled || !videoUrl.trim()} onClick={() => void runJob(`保存 ${side} 视频`, () => savePairwiseMaterials({ taskId, side, videoUrl: videoUrl.trim(), recordingError: '' }))}>
            <Save className="h-4 w-4" />保存 {side} 视频
          </button>
          <span className={`inline-flex h-7 items-center gap-1 rounded-full px-2.5 text-xs font-semibold ${readyVideo ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-300' : 'bg-amber-50 text-amber-700 dark:bg-amber-950/30 dark:text-amber-300'}`}>
            {readyVideo && <CheckCircle2 className="h-3.5 w-3.5" />}{readyVideo ? '视频已就绪' : '待人工补录'}
          </span>
        </div>
      </div>
    </section>
  );
}

export function PairwiseWorkspace({ annotationCase, disabled, runJob }: Props) {
  const pairwise = annotationCase.pairwise;
  if (!pairwise) return null;
  const latestReview = useMemo(() => pairwise.reviews.at(-1), [pairwise.reviews]);
  const currentReview = [...pairwise.reviews].reverse().find((review) => review.current !== false && review.status === 'ready');
  const videosReady = pairwise.runA.videoStatus === 'ready' && pairwise.runB.videoStatus === 'ready';
  const reviewReady = Boolean(pairwise.runA.captureId && pairwise.runB.captureId && pairwise.runA.deliverableSha && pairwise.runB.deliverableSha);

  return (
    <div className="space-y-4">
      <section className="border border-stone-200 bg-white p-4 dark:border-stone-700 dark:bg-stone-900">
        <div className="grid gap-3 text-sm md:grid-cols-4">
          <div><p className="text-xs text-stone-400">Harness</p><p className="mt-1 font-semibold text-stone-700 dark:text-stone-200">{pairwise.harness || '未填写'}</p></div>
          <div><p className="text-xs text-stone-400">版本</p><p className="mt-1 font-semibold text-stone-700 dark:text-stone-200">{pairwise.harnessVersion || '未填写'}</p></div>
          <div><p className="text-xs text-stone-400">操作系统</p><p className="mt-1 font-semibold text-stone-700 dark:text-stone-200">{pairwise.os || '未填写'}</p></div>
          <div><p className="text-xs text-stone-400">初始 SHA</p><p className="mt-1 font-mono text-stone-700 dark:text-stone-200">{shortSha(annotationCase.initialSha)}</p></div>
        </div>
      </section>

      <div className="grid gap-4 xl:grid-cols-2">
        <RunPanel taskId={annotationCase.taskId} side="A" run={pairwise.runA} disabled={disabled} runJob={runJob} />
        <RunPanel taskId={annotationCase.taskId} side="B" run={pairwise.runB} disabled={disabled} runJob={runJob} />
      </div>

      <section className="border border-stone-200 bg-white p-4 dark:border-stone-700 dark:bg-stone-900">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h3 className="text-base font-bold text-stone-900 dark:text-stone-100">GSB 对比结果</h3>
            <p className="mt-1 text-xs text-stone-500">AI 综合两侧轨迹、代码产物和已提供材料生成。</p>
          </div>
          <button className={PRIMARY} disabled={disabled || !reviewReady} onClick={() => void runJob(currentReview ? '重新生成 GSB' : '生成 GSB', () => reviewPairwise({ taskId: annotationCase.taskId, force: Boolean(currentReview) }))}>
            {disabled ? <Loader2 className="h-4 w-4 animate-spin" /> : <Sparkles className="h-4 w-4" />}{currentReview ? '重新生成 GSB' : '生成 GSB'}
          </button>
        </div>
        {!videosReady && <p className="mt-3 text-sm text-amber-700 dark:text-amber-300">当前结论将保持临时状态，正式导出前需补齐 A/B 视频。</p>}
        {latestReview ? (
          <div className="mt-4 border-t border-stone-100 pt-4 dark:border-stone-800">
            <div className="flex flex-wrap items-center gap-2">
              <span className="rounded-full bg-slate-100 px-2.5 py-1 text-xs font-bold text-slate-700 dark:bg-slate-800 dark:text-slate-200">{({ A_better: 'A 更好', B_better: 'B 更好', same: 'Same' } as const)[latestReview.conclusion]}</span>
              {latestReview.current === false && <span className="text-xs font-semibold text-amber-600">证据已变化，需重新生成</span>}
            </div>
            <p className="mt-3 whitespace-pre-wrap text-sm leading-6 text-stone-700 dark:text-stone-300">{latestReview.reason}</p>
          </div>
        ) : <p className="mt-4 text-sm text-stone-500">A/B 证据和产物 commit 齐全后可生成对比结论。</p>}
      </section>
    </div>
  );
}
