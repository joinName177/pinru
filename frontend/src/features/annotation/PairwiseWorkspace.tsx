import { CheckCircle2, Clipboard, Container, Download, ExternalLink, FileSearch, GitBranch, Loader2, Play, RefreshCw, Save, Sparkles, Square } from 'lucide-react';
import { useEffect, useState } from 'react';
import {
  capturePairwiseSide,
  commitPairwiseSide,
  reviewPairwise,
  savePairwiseMaterials,
  type AnnotationCase,
  type AnnotationContainer,
  type AnnotationExportResult,
  type AnnotationPreflightReport,
  type PairwiseRun,
  type PairwiseProjectState,
  type PairwiseSide,
} from '../../api/annotation';
import type { BackgroundJob } from '../../api/job';

const INPUT = 'w-full rounded-lg border border-stone-200 bg-white px-3 py-2 text-sm text-stone-800 outline-none focus:border-slate-400 focus:ring-2 focus:ring-slate-200 dark:border-stone-700 dark:bg-[#171B22] dark:text-stone-100';
const PRIMARY = 'inline-flex h-9 items-center justify-center gap-2 rounded-lg bg-slate-800 px-3 text-sm font-semibold text-white hover:bg-slate-700 disabled:cursor-not-allowed disabled:opacity-40 dark:bg-slate-100 dark:text-slate-900';
const SECONDARY = 'inline-flex h-9 items-center justify-center gap-2 rounded-lg border border-stone-200 bg-white px-3 text-sm font-semibold text-stone-700 hover:bg-stone-50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-stone-700 dark:bg-stone-800 dark:text-stone-200';
const COMPLETED = 'inline-flex h-9 items-center justify-center gap-2 rounded-lg border border-emerald-300 bg-emerald-50 px-3 text-sm font-semibold text-emerald-700 hover:bg-emerald-100 disabled:cursor-not-allowed disabled:opacity-40 dark:border-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-300';

export type PairwiseRunJob = (label: string, submit: () => Promise<BackgroundJob>) => Promise<boolean>;

type Props = {
  annotationCase: AnnotationCase;
  containers?: AnnotationContainer[];
  disabled: boolean;
  runJob: PairwiseRunJob;
  onCopyContainerCommand?: (side: PairwiseSide) => Promise<void>;
  onBindContainer?: (side: PairwiseSide, containerId: string) => Promise<boolean>;
  onRefreshContainers?: (side: PairwiseSide) => Promise<void>;
  onCopyPrompt?: (side: PairwiseSide) => Promise<boolean>;
  onStartProject?: (side: PairwiseSide) => Promise<PairwiseProjectState | null>;
  onStopProject?: (side: PairwiseSide) => Promise<boolean>;
  promptCopied?: Record<PairwiseSide, boolean>;
  onExport?: (draft: boolean) => Promise<void>;
  onPreflight?: () => Promise<void>;
  preflightReport?: AnnotationPreflightReport | null;
  exportResult?: AnnotationExportResult | null;
};

function shortSha(value: string) {
  return value ? value.slice(0, 10) : '尚未提交';
}

function RunPanel({ taskId, side, run, containers, disabled, runJob, onCopyContainerCommand, onBindContainer, onRefreshContainers, onCopyPrompt, onStartProject, onStopProject, promptCopied }: {
  taskId: string;
  side: PairwiseSide;
  run: PairwiseRun;
  containers: AnnotationContainer[];
  disabled: boolean;
  runJob: PairwiseRunJob;
  onCopyContainerCommand?: (side: PairwiseSide) => Promise<void>;
  onBindContainer?: (side: PairwiseSide, containerId: string) => Promise<boolean>;
  onRefreshContainers?: (side: PairwiseSide) => Promise<void>;
  onCopyPrompt?: (side: PairwiseSide) => Promise<boolean>;
  onStartProject?: (side: PairwiseSide) => Promise<PairwiseProjectState | null>;
  onStopProject?: (side: PairwiseSide) => Promise<boolean>;
  promptCopied: boolean;
}) {
  const [videoSource, setVideoSource] = useState(run.videoPath || run.videoUrl || '');
  const [containerId, setContainerId] = useState(run.containerId || '');
  const [refreshing, setRefreshing] = useState(false);
  const [projectBusy, setProjectBusy] = useState(false);
  const [projectURL, setProjectURL] = useState('');
  useEffect(() => setVideoSource(run.videoPath || run.videoUrl || ''), [run.videoPath, run.videoUrl]);
  useEffect(() => setContainerId(run.containerId || ''), [run.containerId]);
  const readyVideo = run.videoStatus === 'ready';
  const boundSelected = Boolean(containerId && containerId === run.containerId);
  const showBound = boundSelected;

  return (
    <section role="region" aria-label={`运行 ${side}`} className="border border-stone-200 bg-white p-4 dark:border-stone-700 dark:bg-stone-900">
      <div className="border-b border-stone-100 pb-3 dark:border-stone-800">
        <div>
          <h3 className="text-base font-bold text-stone-900 dark:text-stone-100">运行 {side}</h3>
          <p className="mt-1 text-xs text-stone-500">固定分支 {side} · {run.preparedAt ? '已从初始快照准备' : '绑定容器时自动准备'}</p>
        </div>
      </div>

      <div className="mt-3 grid gap-2 sm:grid-cols-[1fr_auto_auto]">
        <select aria-label={`${side} 容器`} className={INPUT} value={containerId} onChange={(event) => setContainerId(event.target.value)}>
          <option value="">选择 {side} 独立容器</option>
          {containers.map((container) => <option key={container.id} value={container.id}>{container.name} · {container.state}</option>)}
        </select>
        <button className={SECONDARY} disabled={disabled || refreshing} onClick={() => {
          setRefreshing(true);
          void Promise.resolve(onRefreshContainers?.(side)).finally(() => setRefreshing(false));
        }}>{refreshing ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}刷新容器</button>
        <button className={showBound ? COMPLETED : SECONDARY} disabled={disabled || !containerId} onClick={() => void onBindContainer?.(side, containerId)}>{showBound ? <CheckCircle2 className="h-4 w-4" /> : <Container className="h-4 w-4" />}{showBound ? `已绑定 ${side}` : `绑定 ${side}`}</button>
      </div>
      <div className="mt-2 flex flex-wrap gap-2">
        <button className={SECONDARY} disabled={disabled} onClick={() => void onCopyContainerCommand?.(side)}><Clipboard className="h-4 w-4" />复制 {side} 启动命令</button>
        <button className={promptCopied ? COMPLETED : SECONDARY} disabled={disabled || !run.containerId} onClick={() => void onCopyPrompt?.(side)}>{promptCopied ? <CheckCircle2 className="h-4 w-4" /> : <Clipboard className="h-4 w-4" />}{promptCopied ? '提示词已复制' : '复制同一提示词'}</button>
      </div>

      <dl className="mt-3 grid grid-cols-2 gap-3 text-xs">
        <div><dt className="text-stone-400">SessionID</dt><dd className="mt-1 break-all font-mono text-stone-700 dark:text-stone-300">{run.sessionId || '尚未采集'}</dd></div>
        <div><dt className="text-stone-400">产物 commit</dt><dd className="mt-1 font-mono text-stone-700 dark:text-stone-300">{shortSha(run.deliverableSha)}</dd></div>
      </dl>

      <div className="mt-4 space-y-3">
        <div className="flex flex-wrap gap-2">
          <button className={PRIMARY} disabled={disabled || !run.containerId} onClick={() => void runJob(`采集 ${side}`, () => capturePairwiseSide({ taskId, side }))}>
            <FileSearch className="h-4 w-4" />采集 {side}
          </button>
          <button className={run.deliverableSha ? COMPLETED : SECONDARY} disabled={disabled || !run.sessionId} onClick={() => void runJob(`提交 ${side} 产物`, () => commitPairwiseSide({ taskId, side, sessionId: run.sessionId }))}>
            {run.deliverableSha ? <CheckCircle2 className="h-4 w-4" /> : <GitBranch className="h-4 w-4" />}{run.deliverableSha ? `已提交 ${side}` : `提交 ${side} 产物`}
          </button>
        </div>

        <div className="flex flex-wrap items-center gap-2 border-t border-stone-100 pt-3 dark:border-stone-800">
          <button aria-label={`启动项目 ${side}`} className={PRIMARY} disabled={disabled || projectBusy || !run.deliverableSha || !onStartProject} onClick={() => {
            setProjectBusy(true);
            void Promise.resolve(onStartProject?.(side) ?? null).then((state) => {
              if (state?.running) setProjectURL(state.url);
            }).finally(() => setProjectBusy(false));
          }}>
            {projectBusy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}启动项目
          </button>
          <button aria-label={`停止项目 ${side}`} className={SECONDARY} disabled={disabled || projectBusy || !run.containerId || !onStopProject} onClick={() => {
            setProjectBusy(true);
            void Promise.resolve(onStopProject?.(side) ?? false).then((stopped) => {
              if (stopped) setProjectURL('');
            }).finally(() => setProjectBusy(false));
          }}>
            <Square className="h-4 w-4" />停止项目
          </button>
          {projectURL && <a aria-label={`打开 ${side} 项目`} className="inline-flex h-9 items-center gap-1 text-sm font-semibold text-sky-700 hover:underline dark:text-sky-300" href={projectURL} target="_blank" rel="noreferrer"><ExternalLink className="h-4 w-4" />打开项目</a>}
        </div>

        <label className="block">
          <span className="mb-1 block text-xs font-semibold text-stone-500">运行录屏文件</span>
          <input aria-label={`${side} 视频链接`} className={INPUT} value={videoSource} onChange={(event) => setVideoSource(event.target.value)} placeholder="/absolute/path/to/recording.mp4（也兼容 HTTP(S) 链接）" />
        </label>
        <div className="flex flex-wrap items-center gap-2">
          <button className={SECONDARY} disabled={disabled || !videoSource.trim()} onClick={() => {
            const source = videoSource.trim();
            return void runJob(`保存 ${side} 视频`, () => savePairwiseMaterials({ taskId, side, videoUrl: /^https?:\/\//.test(source) ? source : '', videoPath: /^https?:\/\//.test(source) ? '' : source, recordingError: '' }));
          }}>
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

export function PairwiseWorkspace({ annotationCase, containers = [], disabled, runJob, onCopyContainerCommand, onBindContainer, onRefreshContainers, onCopyPrompt, onStartProject, onStopProject, promptCopied = { A: false, B: false }, onExport, onPreflight, preflightReport, exportResult }: Props) {
  const pairwise = annotationCase.pairwise;
  if (!pairwise) return null;
  const latestReview = pairwise.reviews.at(-1);
  const currentReview = [...pairwise.reviews].reverse().find((review) => review.current === true && review.status === 'ready');
  const videosReady = pairwise.runA.videoStatus === 'ready' && pairwise.runB.videoStatus === 'ready';
  const reviewReady = Boolean(pairwise.runA.captureId && pairwise.runB.captureId && pairwise.runA.deliverableSha && pairwise.runB.deliverableSha);
  const formalExportReady = videosReady && Boolean(currentReview);
  return (
    <div className="space-y-4">
      <div className="grid gap-4 xl:grid-cols-2">
        <RunPanel taskId={annotationCase.taskId} side="A" run={pairwise.runA} containers={containers} disabled={disabled} runJob={runJob} onCopyContainerCommand={onCopyContainerCommand} onBindContainer={onBindContainer} onRefreshContainers={onRefreshContainers} onCopyPrompt={onCopyPrompt} onStartProject={onStartProject} onStopProject={onStopProject} promptCopied={promptCopied.A} />
        <RunPanel taskId={annotationCase.taskId} side="B" run={pairwise.runB} containers={containers} disabled={disabled} runJob={runJob} onCopyContainerCommand={onCopyContainerCommand} onBindContainer={onBindContainer} onRefreshContainers={onRefreshContainers} onCopyPrompt={onCopyPrompt} onStartProject={onStartProject} onStopProject={onStopProject} promptCopied={promptCopied.B} />
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
              {latestReview.current !== true && <span className="text-xs font-semibold text-amber-600">证据已变化，需重新生成</span>}
            </div>
            <p className="mt-3 whitespace-pre-wrap text-sm leading-6 text-stone-700 dark:text-stone-300">{latestReview.reason}</p>
          </div>
        ) : <p className="mt-4 text-sm text-stone-500">A/B 证据和产物 commit 齐全后可生成对比结论。</p>}
      </section>

      {onExport && <section className="border border-stone-200 bg-white p-4 dark:border-stone-700 dark:bg-stone-900">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h3 className="text-base font-bold text-stone-900 dark:text-stone-100">Pair-wise 导出</h3>
            <p className="mt-1 text-xs text-stone-500">草稿保留待补项；正式导出要求 A/B 视频和当前 GSB 均已就绪。</p>
          </div>
          {onPreflight && <button className={SECONDARY} disabled={disabled} onClick={() => void onPreflight()}><FileSearch className="h-4 w-4" />Pair-wise 预检</button>}
        </div>
        {preflightReport && <div className={`mt-4 border p-3 text-sm ${preflightReport.issues.length ? 'border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-900 dark:bg-amber-950/20 dark:text-amber-200' : 'border-emerald-200 bg-emerald-50 text-emerald-800 dark:border-emerald-900 dark:bg-emerald-950/20 dark:text-emerald-200'}`}>
          <p className="font-semibold">{preflightReport.issues.length ? `仍有 ${preflightReport.issues.length} 项待补` : '正式导出材料齐全'}</p>
          {preflightReport.issues.length > 0 && <ul className="mt-2 space-y-1 text-xs">{preflightReport.issues.map((issue) => <li key={issue}>• {issue}</li>)}</ul>}
        </div>}
        <div className="mt-3 flex flex-wrap gap-2">
          <button className={SECONDARY} disabled={disabled} onClick={() => void onExport(true)}><Download className="h-4 w-4" />导出 Pair-wise 草稿</button>
          <button className={PRIMARY} disabled={disabled || !formalExportReady} onClick={() => void onExport(false)}><Download className="h-4 w-4" />正式导出 Pair-wise</button>
        </div>
        {exportResult && <div className="mt-4 border border-emerald-200 bg-emerald-50 p-3 text-xs text-emerald-800 dark:border-emerald-900 dark:bg-emerald-950/20 dark:text-emerald-200">
          <p className="font-semibold">已导出 {exportResult.rows} 行</p>
          <p className="mt-1 break-all">Excel：{exportResult.outputPath}</p>
          <p className="mt-1 break-all">报告：{exportResult.reportPath}</p>
        </div>}
      </section>}
    </div>
  );
}
