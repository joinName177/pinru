import {
  AlertCircle,
  Archive,
  CheckCircle2,
  Clipboard,
  Container,
  Download,
  FileSearch,
  Loader2,
  RefreshCw,
  Save,
  Sparkles,
  Square,
} from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  bindContainer,
  bindPairwiseContainer,
  batchCaptureAndPrepareTable,
  batchReviewPairwise,
  cancelAnnotationJob,
  captureAndPrepareTable,
  enablePairwise,
  exportCases,
  exportPairwise,
  listCases,
  listContainers,
  listTraces,
  preflight,
  preflightPairwise,
  prepareCase,
  publishSnapshot,
  reviewRound,
  resumeTable,
  saveCaseSettings,
  startPairwiseProject,
  stopPairwiseProject,
  type AnnotationCase,
  type AnnotationBatchPrepareResult,
  type AnnotationContainer,
  type AnnotationEvaluation,
  type AnnotationExportResult,
  type AnnotationPreflightReport,
  type AnnotationRound,
  type PairwiseBatchReviewResult,
  type TraceCandidate,
} from '../../api/annotation';
import type { BackgroundJob } from '../../api/job';
import { useAppStore } from '../../store';
import { writeClipboardText } from '../../shared/lib/clipboard';
import { waitForAnnotationJob } from './job';
import { buildContainerCommand } from './containerCommand';
import { getAnnotationContainerApiKey, getConfig } from '../../api/config';
import {
  CONTAINER_SORT_PREFIXES_CONFIG_KEY,
  sortContainersByPrefixes,
} from './containerSorting';
import { evaluationScoreTotal, getTableProgress, latestEvaluation } from './tableProgress';
import { TableStatusBadge } from './TableStatusBadge';
import { CrossProjectBatchSelector } from './CrossProjectBatchSelector';
import { PairwiseWorkspace } from './PairwiseWorkspace';

const INPUT_CLASS = 'w-full rounded-xl border border-stone-200 bg-white px-3 py-2 text-sm text-stone-800 outline-none transition focus:border-slate-400 focus:ring-2 focus:ring-slate-200 dark:border-stone-700 dark:bg-[#171B22] dark:text-stone-100 dark:focus:border-slate-500 dark:focus:ring-slate-800';
const PRIMARY_BUTTON = 'inline-flex items-center justify-center gap-2 rounded-xl bg-slate-800 px-3.5 py-2 text-sm font-semibold text-white transition hover:bg-slate-700 disabled:cursor-not-allowed disabled:opacity-40 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white';
const SECONDARY_BUTTON = 'inline-flex items-center justify-center gap-2 rounded-xl border border-stone-200 bg-white px-3.5 py-2 text-sm font-semibold text-stone-700 transition hover:bg-stone-50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-stone-700 dark:bg-stone-800 dark:text-stone-200 dark:hover:bg-stone-700';
const COMPLETED_BUTTON = 'inline-flex items-center justify-center gap-2 rounded-xl border border-emerald-300 bg-emerald-50 px-3.5 py-2 text-sm font-semibold text-emerald-700 transition hover:bg-emerald-100 disabled:cursor-not-allowed disabled:opacity-40 dark:border-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-300 dark:hover:bg-emerald-950/60';

const SCORE_DIMENSIONS = ['交付完整性', '指令遵循', '任务规划', '推理能力', '执行能力'];

type BusyAction = {
  taskId: string;
  label: string;
  jobId: string;
  progress: number;
  message: string;
};

type AnnotationWorkspaceProps = {
  projectId: string;
  projectName?: string;
  taskId?: string;
  view?: 'capture' | 'review';
  promptText?: string;
  onPromptCopy?: () => void | Promise<void>;
};

function folderName(path: string) {
  const normalized = path.replace(/[\\/]+$/, '');
  return normalized.split(/[\\/]/).pop() || 'repository';
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

function statusLabel(status: string) {
  return ({ complete: '轨迹完整', pending: '会话未结束', conflict: '轨迹有冲突', excluded: '已排除' } as Record<string, string>)[status] ?? status;
}

function formatBytes(size: number) {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}

function parseJobOutput<T>(job: BackgroundJob, fallback: string): T {
  if (!job.outputPayload) throw new Error(fallback);
  try {
    return JSON.parse(job.outputPayload) as T;
  } catch {
    throw new Error(`${fallback}：后台返回了无法读取的结果`);
  }
}

function evaluationsFor(round: AnnotationRound) {
  return round.evaluations ?? [];
}

function isPerfectEvaluation(evaluation: AnnotationEvaluation) {
  return evaluation.scores.length === 5 && evaluation.scores.every((score) => score === 5);
}

function ScoreGrid({ evaluation }: { evaluation: AnnotationEvaluation }) {
  return (
    <div className="grid gap-2 md:grid-cols-5">
      {SCORE_DIMENSIONS.map((dimension, index) => {
        const score = evaluation.scores[index] ?? null;
        return (
          <div key={dimension} className="rounded-xl border border-stone-200 bg-stone-50 p-3 dark:border-stone-700 dark:bg-stone-800/60">
            <div className="flex items-center justify-between gap-2">
              <span className="text-xs font-semibold text-stone-500 dark:text-stone-400">{dimension}</span>
              <span className={`text-xs font-bold ${score === null ? 'text-amber-600 dark:text-amber-400' : 'text-stone-800 dark:text-stone-100'}`}>
                {score === null ? '待补证据' : `${score} 分`}
              </span>
            </div>
            <p className="mt-2 text-xs leading-5 text-stone-600 dark:text-stone-300">
              {evaluation.descriptions[index] || '暂无说明'}
            </p>
          </div>
        );
      })}
    </div>
  );
}

function EvaluationCard({ evaluation, index }: { evaluation: AnnotationEvaluation; index: number; key?: string }) {
  const scoreTotal = evaluationScoreTotal(evaluation);
  return (
    <div className="rounded-2xl border border-stone-200 bg-white p-4 dark:border-stone-700 dark:bg-stone-900/60">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div>
          <h4 className="text-sm font-semibold text-stone-800 dark:text-stone-100">评价版本 {index + 1}</h4>
          <p className="mt-0.5 text-[11px] text-stone-400">
            {evaluation.model || '未知模型'} · skill {evaluation.skillHash || '未记录'} · 证据 {evaluation.evidenceHash || '未记录'}
          </p>
        </div>
        <span className="rounded-full bg-stone-100 px-2.5 py-1 text-[11px] font-semibold text-stone-600 dark:bg-stone-800 dark:text-stone-300">
          {evaluation.current === false ? '历史评价 · 待重新复审' : evaluation.status === 'needs_evidence' ? '审核待补证据' : scoreTotal !== null && scoreTotal > 21 ? `五维总分 ${scoreTotal} · 超过21，不收录` : isPerfectEvaluation(evaluation) ? '五维满分 · 数据已保存' : `五维总分 ${scoreTotal ?? '-'} · 可收录`}
        </span>
      </div>
      <ScoreGrid evaluation={evaluation} />
      <p className="mt-3 text-sm text-stone-700 dark:text-stone-300">
        {evaluation.issues.some((issue) => issue.kind === 'bug')
          ? '发现待修复问题：请查看下方问题依据与修复提示词。'
          : evaluation.status === 'needs_evidence' || evaluation.missing.length > 0
            ? '证据不足，暂不能确认是否需要修复；未凭缺少验证记录编造缺陷。'
            : '本轮审核未发现待修复的代码问题，因此未生成修复提示词。过程扣分不等于功能未完成，具体核验范围见证据。'}
      </p>

      <div className="mt-3 rounded-xl bg-stone-50 p-3 dark:bg-stone-800/50">
        <p className="text-xs font-semibold text-stone-500">逐项需求核验</p>
        {evaluation.requirementChecks?.length ? <ul className="mt-2 space-y-3 text-xs text-stone-700 dark:text-stone-300">
          {evaluation.requirementChecks.map((check,i)=><li key={i}>
            <span className="font-semibold">{({completed:'已完成',failed:'存在未解决问题',unverified:'尚未核实'})[check.status]} · {check.requirement}</span>
            <p className="mt-1 whitespace-pre-wrap break-words text-stone-500">{check.evidence}</p>
          </li>)}
        </ul> : <p className="mt-2 text-xs text-stone-500">旧版评价未记录逐项核验清单；可查看已有依据，重新审核后补充。</p>}
      </div>

      <div className="mt-3 grid gap-3 lg:grid-cols-3">
        <div className="rounded-xl bg-stone-50 p-3 dark:bg-stone-800/50">
          <p className="text-xs font-semibold text-stone-500 dark:text-stone-400">证据来源</p>
          {evaluation.evidence.length ? (
            <ul className="mt-2 space-y-1 text-xs text-stone-700 dark:text-stone-300">
              {evaluation.evidence.map((item, itemIndex) => <li key={`${item}-${itemIndex}`}>{item}</li>)}
            </ul>
          ) : <p className="mt-2 text-xs text-stone-400">未记录</p>}
        </div>
        <div className="rounded-xl bg-amber-50 p-3 dark:bg-amber-950/20">
          <p className="text-xs font-semibold text-amber-700 dark:text-amber-300">材料缺项</p>
          {evaluation.missing.length ? (
            <ul className="mt-2 space-y-1 text-xs text-amber-800 dark:text-amber-200">
              {evaluation.missing.map((item) => <li key={item}>{item}</li>)}
            </ul>
          ) : <p className="mt-2 text-xs text-amber-700/70 dark:text-amber-300/70">无</p>}
        </div>
        <div className="rounded-xl bg-red-50 p-3 dark:bg-red-950/20">
          <p className="text-xs font-semibold text-red-700 dark:text-red-300">发现的问题</p>
          {evaluation.issues.length ? (
            <ul className="mt-2 space-y-2 text-xs text-red-800 dark:text-red-200">
              {evaluation.issues.map((issue, issueIndex) => (
                <li key={`${issue.kind}-${issueIndex}`}>
                  <span className="font-semibold">{issue.description}</span>
                  {issue.evidence && <span className="block text-red-600/80 dark:text-red-300/70">{issue.evidence}</span>}
                </li>
              ))}
            </ul>
          ) : <p className="mt-2 text-xs text-red-700/70 dark:text-red-300/70">无</p>}
        </div>
      </div>
      {evaluation.limitations?.length ? (
        <div className="mt-3 rounded-xl bg-blue-50 p-3 dark:bg-blue-950/20">
          <p className="text-xs font-semibold text-blue-700 dark:text-blue-300">验证边界（不阻塞制表）</p>
          <ul className="mt-2 space-y-1 text-xs text-blue-800 dark:text-blue-200">
            {evaluation.limitations.map((item) => <li key={item}>{item}</li>)}
          </ul>
        </div>
      ) : null}
    </div>
  );
}

export function AnnotationWorkspace({ projectId, projectName, taskId, view = 'capture', promptText = '', onPromptCopy }: AnnotationWorkspaceProps) {
  const [cases, setCases] = useState<AnnotationCase[]>([]);
  const [containers, setContainers] = useState<AnnotationContainer[]>([]);
  const [selectedTaskId, setSelectedTaskId] = useState('');
  const [selectedContainerId, setSelectedContainerId] = useState('');
  const [repoRelativePath, setRepoRelativePath] = useState('');
  const [traceCandidates, setTraceCandidates] = useState<TraceCandidate[]>([]);
  const [selectedTracePath, setSelectedTracePath] = useState('');
  const [snapshotUrl, setSnapshotUrl] = useState('');
  const [completed, setCompleted] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState('');
  const [actionError, setActionError] = useState('');
  const [notice, setNotice] = useState('');
  const [caseBusy, setCaseBusy] = useState<Record<string, BusyAction>>({});
  const [saving, setSaving] = useState(false);
  const [report, setReport] = useState<AnnotationPreflightReport | null>(null);
  const [pairwiseReport, setPairwiseReport] = useState<AnnotationPreflightReport | null>(null);
  const [preflightLoading, setPreflightLoading] = useState(false);
  const [submitter, setSubmitter] = useState('');
  const [submittedAt, setSubmittedAt] = useState('');
  const [exportResult, setExportResult] = useState<AnnotationExportResult | null>(null);
  const [exportBusy, setExportBusy] = useState<BusyAction | null>(null);
  const [batchBusy, setBatchBusy] = useState<BusyAction | null>(null);
  const [batchResult, setBatchResult] = useState<AnnotationBatchPrepareResult | null>(null);
  const [pairwiseBatchResult, setPairwiseBatchResult] = useState<PairwiseBatchReviewResult | null>(null);
  const [showBatchSelection, setShowBatchSelection] = useState(false);
  const [showExportSelection, setShowExportSelection] = useState(false);
  const [exportTaskIds, setExportTaskIds] = useState<string[]>([]);
  const [startupCommandCopied, setStartupCommandCopied] = useState(false);
  const [bindingCompleted, setBindingCompleted] = useState(false);
  const [quickPromptCopied, setQuickPromptCopied] = useState(false);
  const [pairwisePromptCopied, setPairwisePromptCopied] = useState<Record<'A' | 'B', boolean>>({ A: false, B: false });
  const exportableCases = cases.filter((item) => getTableProgress(item).prepared > 0);
  const pairwiseCases = cases.filter((item) => item.mode === 'pairwise_gsb');
  const legacyCases = cases.filter((item) => item.mode !== 'pairwise_gsb');
  const eligibleExportIds = exportTaskIds.filter((id) => exportableCases.some((item) => item.taskId === id));
  const projectEpoch = useRef(0);
  const settingsEpoch = useRef(0);
  const activeProjectId = useRef(projectId);

  const selectedCase = useMemo(
    () => cases.find((item) => item.taskId === (taskId ?? selectedTaskId)) ?? null,
    [cases, selectedTaskId, taskId],
  );
  const selectedProgress = selectedCase ? getTableProgress(selectedCase) : null;
  const persistedJob = selectedCase?.preparation;
  const persistedBusy: BusyAction | null = selectedCase && persistedJob && ['pending','running'].includes(persistedJob.status)
    ? {taskId:selectedCase.taskId,label:'准备制表数据',jobId:persistedJob.jobId,progress:persistedJob.progress,message:persistedJob.message} : null;
  const selectedBusy = selectedCase ? caseBusy[selectedCase.taskId] ?? persistedBusy : null;
  const startup = useMemo(() => {
    if (!selectedCase) return null;
    try { return { value: buildContainerCommand(selectedCase), error: '' }; }
    catch (error) { return { value: null, error: errorMessage(error) }; }
  }, [selectedCase?.taskId, selectedCase?.taskName, selectedCase?.sourcePath]);
  const visibleBusy = batchBusy ?? exportBusy ?? selectedBusy ?? Object.values(caseBusy)[0] ?? null;
  const globalBusy = Boolean(batchBusy || exportBusy);

  const loadProject = useCallback(async (targetProjectId: string) => {
    const epoch = ++projectEpoch.current;
    setLoading(true);
    setLoadError('');
    try {
      const [caseResult, containerResult, prefixResult] = await Promise.allSettled([
        listCases(targetProjectId),
        listContainers(),
        getConfig(CONTAINER_SORT_PREFIXES_CONFIG_KEY),
      ]);
      if (epoch !== projectEpoch.current || targetProjectId !== activeProjectId.current) return null;
      const nextCases = caseResult.status === 'fulfilled' ? caseResult.value : [];
      const nextContainers = containerResult.status === 'fulfilled' ? containerResult.value : [];
      const containerSortPrefixes = prefixResult.status === 'fulfilled' ? prefixResult.value : 'cyc';
      const sortedContainers = sortContainersByPrefixes(nextContainers, containerSortPrefixes);
      setCases(nextCases);
      setContainers(sortedContainers);
      setSelectedTaskId((current) => nextCases.some((item) => item.taskId === current) ? current : (nextCases[0]?.taskId ?? ''));
      const errors = [
        caseResult.status === 'rejected' ? `题目加载失败：${errorMessage(caseResult.reason)}` : '',
        containerResult.status === 'rejected' ? `容器列表加载失败：${errorMessage(containerResult.reason)}` : '',
        prefixResult.status === 'rejected' ? `容器排序设置加载失败：${errorMessage(prefixResult.reason)}` : '',
      ].filter(Boolean);
      setLoadError(errors.join('；'));
      return { cases: nextCases, containers: sortedContainers };
    } catch (error) {
      if (epoch !== projectEpoch.current || targetProjectId !== activeProjectId.current) return null;
      setLoadError(errorMessage(error));
      setCases([]);
      return null;
    } finally {
      if (epoch === projectEpoch.current && targetProjectId === activeProjectId.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    activeProjectId.current = projectId;
    setReport(null);
    setPairwiseReport(null);
    setExportResult(null);
    setCaseBusy({});
    setExportBusy(null);
    setBatchBusy(null);
    setBatchResult(null);
    setPairwiseBatchResult(null);
    setShowBatchSelection(false);
    setShowExportSelection(false);
    setExportTaskIds([]);
    setSaving(false);
    setPreflightLoading(false);
    setStartupCommandCopied(false);
    setBindingCompleted(false);
    setQuickPromptCopied(false);
    setPairwisePromptCopied({ A: false, B: false });
    settingsEpoch.current += 1;
    setSelectedTaskId('');
    void loadProject(projectId);
    return () => { projectEpoch.current += 1; };
  }, [loadProject, projectId]);

  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    const refresh = async () => {
      try {
        const result = await listCases(projectId);
        if (active && activeProjectId.current === projectId) setCases(result);
      } catch { /* The last known state remains visible; manual refresh reports errors. */ }
      finally { if (active) timer=setTimeout(refresh,3000); }
    };
    timer=setTimeout(refresh,3000);
    return () => { active=false; clearTimeout(timer); };
  }, [projectId]);

  useEffect(() => {
    if (!selectedCase) return;
    setSelectedContainerId(selectedCase.containerId || '');
    setRepoRelativePath(selectedCase.repoRelativePath || folderName(selectedCase.sourcePath));
    setSnapshotUrl(selectedCase.snapshotUrl || '');
    setCompleted(selectedCase.completed);
    setSelectedTracePath(selectedCase.tracePath || '');
  }, [selectedCase?.taskId, selectedCase?.revision]);

  useEffect(() => {
    setStartupCommandCopied(false);
    setBindingCompleted(false);
    setQuickPromptCopied(false);
    setPairwisePromptCopied({ A: false, B: false });
  }, [selectedCase?.taskId]);

  useEffect(() => {
    setQuickPromptCopied(false);
  }, [promptText]);

  useEffect(() => {
    setNotice('');
    setActionError('');
  }, [selectedCase?.taskId]);

  useEffect(() => {
    if (loading) return;
    if (!selectedCase?.containerId) {
      setTraceCandidates([]);
      return;
    }
    let current = true;
    const taskId = selectedCase.taskId;
    void listTraces(taskId)
      .then((traces) => {
        if (!current || selectedCase.taskId !== taskId) return;
        setTraceCandidates(traces);
        setSelectedTracePath((currentPath) => {
          if (currentPath && currentPath !== selectedCase.tracePath) return currentPath;
          if (selectedCase.tracePath && traces.some((trace) => trace.path === selectedCase.tracePath)) return selectedCase.tracePath;
          if (traces.length === 1) return traces[0].path;
          return '';
        });
      })
      .catch((error) => { if (current) setActionError(errorMessage(error)); });
    return () => { current = false; };
  }, [loading, selectedCase?.containerId, selectedCase?.taskId, selectedCase?.tracePath, selectedCase?.revision]);

  const replaceCase = useCallback((updated: AnnotationCase) => {
    if (updated.projectId && updated.projectId !== activeProjectId.current) return;
    setCases((current) => current.map((item) => item.taskId === updated.taskId ? updated : item));
    setReport(null);
    setPairwiseReport(null);
  }, []);

  const updateCaseBusy = useCallback((taskId: string, next: BusyAction | null) => {
    setCaseBusy((current) => {
      const updated = { ...current };
      if (next) updated[taskId] = next;
      else delete updated[taskId];
      return updated;
    });
  }, []);

  const runCaseJob = useCallback(async (
    taskId: string,
    label: string,
    submit: () => Promise<BackgroundJob>,
  ) => {
    const targetProjectId = activeProjectId.current;
    setActionError('');
    setNotice('');
    setReport(null);
    setPairwiseReport(null);
    updateCaseBusy(taskId, { taskId, label, jobId: '', progress: 0, message: '正在提交后台任务' });
    try {
      const submitted = await submit();
      if (targetProjectId !== activeProjectId.current) return false;
      updateCaseBusy(taskId, { taskId, label, jobId: submitted.id, progress: submitted.progress ?? 0, message: submitted.progressMessage ?? '等待执行' });
      const finished = await waitForAnnotationJob(submitted.id, (job) => {
        if (targetProjectId !== activeProjectId.current) return;
        updateCaseBusy(taskId, { taskId, label, jobId: submitted.id, progress: job.progress, message: job.progressMessage || '执行中' });
      });
      if (targetProjectId !== activeProjectId.current) return false;
      const updated = parseJobOutput<AnnotationCase>(finished, `${label}完成但没有返回题目结果`);
      replaceCase(updated);
      setNotice(`${label}已完成`);
      return true;
    } catch (error) {
      if (targetProjectId === activeProjectId.current) {
        setActionError(errorMessage(error));
        if (label === '采集并准备制表数据') {
          // A failed later round may still have saved the capture and earlier scores.
          try {
            const savedCases = await listCases(targetProjectId);
            const savedCase = savedCases.find((item) => item.taskId === taskId);
            if (targetProjectId === activeProjectId.current && savedCase) replaceCase(savedCase);
          } catch { /* Keep the original job error visible; manual refresh can retry. */ }
        }
      }
      return false;
    } finally {
      if (targetProjectId === activeProjectId.current) updateCaseBusy(taskId, null);
    }
  }, [replaceCase, updateCaseBusy]);

  const handleCopyStartupCommand = async () => {
    if (!selectedCase || !startup?.value) return;
    try {
      const apiKey = await getAnnotationContainerApiKey();
      await writeClipboardText(buildContainerCommand(selectedCase, apiKey).command);
      setStartupCommandCopied(true);
      setNotice('容器启动命令已复制，请在本地终端执行');
      setActionError('');
    } catch (error) {
      setActionError(errorMessage(error));
    }
  };

  const handleCopyPairwiseStartupCommand = async (side: 'A' | 'B') => {
    if (!selectedCase) return;
    const apiKey = await getAnnotationContainerApiKey();
    await writeClipboardText(buildContainerCommand(selectedCase, apiKey, side).command);
    setNotice(`${side} 容器启动命令已复制`);
    setActionError('');
  };

  const copyPairwisePrompt = async (side: 'A' | 'B', afterBinding = false) => {
    const storedPrompt = selectedCase?.pairwise?.prompt.trim() || '';
    const text = promptText.trim() || storedPrompt;
    if (!text) return false;
    try {
      if (promptText.trim() && onPromptCopy) await onPromptCopy();
      else await writeClipboardText(text);
      setPairwisePromptCopied((current) => ({ ...current, [side]: true }));
      setNotice(`${side} 提示词已复制`);
      setActionError('');
      return true;
    } catch (error) {
      const message = errorMessage(error);
      setActionError(afterBinding ? `${side} 绑定成功，但提示词复制失败：${message}` : message);
      return false;
    }
  };

  const bindPairwiseCaseToContainer = async (targetCase: AnnotationCase, side: 'A' | 'B', containerId: string) => {
    const run = side === 'A' ? targetCase.pairwise?.runA : targetCase.pairwise?.runB;
    if (run?.containerId === containerId) {
      await copyPairwisePrompt(side, true);
      return true;
    }
    const completed = await runCaseJob(targetCase.taskId, `绑定 ${side} 容器`, () => bindPairwiseContainer({
      taskId: targetCase.taskId,
      side,
      containerId,
      repoRelativePath: targetCase.repoRelativePath || folderName(targetCase.sourcePath),
      copyRepository: true,
    }));
    if (completed) await copyPairwisePrompt(side, true);
    return completed;
  };

  const handleBindPairwiseContainer = async (side: 'A' | 'B', containerId: string) => {
    if (!selectedCase) return false;
    return bindPairwiseCaseToContainer(selectedCase, side, containerId);
  };

  const handleRefreshPairwiseContainers = async (side: 'A' | 'B') => {
    const refreshed = await loadProject(projectId);
    if (!refreshed || !taskId) return;
    const targetCase = refreshed.cases.find((item) => item.taskId === taskId);
    if (!targetCase) return;
    let expectedName = '';
    try {
      expectedName = buildContainerCommand(targetCase, '', side).containerName.toLowerCase();
    } catch {
      return;
    }
    const matches = refreshed.containers.filter((item) => item.name.toLowerCase() === expectedName);
    if (matches.length !== 1) return;
    const matched = matches[0];
    setPairwisePromptCopied((current) => ({ ...current, [side]: false }));
    if (matched.state !== 'running') {
      setActionError(`已匹配 ${side} 容器 ${matched.name}，但容器未运行，请启动后重试。`);
      return;
    }
    if (!targetCase.initialSha) {
      setActionError(`已匹配 ${side} 容器，但题目尚未准备初始快照。`);
      return;
    }
    await bindPairwiseCaseToContainer(targetCase, side, matched.id);
  };

  const handleStartPairwiseProject = async (side: 'A' | 'B') => {
    if (!selectedCase) return null;
    try {
      const state = await startPairwiseProject({ taskId: selectedCase.taskId, side });
      setNotice(`${side} 项目已从 ${side} 分支的产物提交启动`);
      setActionError('');
      window.open(state.url, '_blank', 'noopener,noreferrer');
      return state;
    } catch (error) {
      setActionError(errorMessage(error));
      return null;
    }
  };

  const handleStopPairwiseProject = async (side: 'A' | 'B') => {
    if (!selectedCase) return false;
    try {
      await stopPairwiseProject({ taskId: selectedCase.taskId, side });
      setNotice(`${side} 项目已停止`);
      setActionError('');
      return true;
    } catch (error) {
      setActionError(errorMessage(error));
      return false;
    }
  };

  const copyCurrentPrompt = async (afterBinding = false) => {
    if (!promptText.trim() || !onPromptCopy) return false;
    try {
      await onPromptCopy();
      setQuickPromptCopied(true);
      setNotice('提示词已复制');
      setActionError('');
      return true;
    } catch (error) {
      const message = errorMessage(error);
      setActionError(afterBinding ? `绑定成功，但提示词复制失败：${message}` : message);
      return false;
    }
  };

  const bindCaseToContainer = async (targetCase: AnnotationCase, containerId: string) => {
    const completed = await runCaseJob(
      targetCase.taskId,
      '复制并绑定',
      () => bindContainer({
        taskId: targetCase.taskId,
        containerId,
        repoRelativePath: targetCase.repoRelativePath || folderName(targetCase.sourcePath),
        copyRepository: true,
      }),
    );
    if (!completed) return false;
    setBindingCompleted(true);
    await copyCurrentPrompt(true);
    return true;
  };

  const handleBindContainer = async () => {
    if (!selectedCase) return;
    await bindCaseToContainer(
      { ...selectedCase, repoRelativePath: repoRelativePath.trim() },
      selectedContainerId,
    );
  };

  const handleQuickPromptCopy = async () => {
    await copyCurrentPrompt(false);
  };

  const handleRefreshContainers = async () => {
    const refreshed = await loadProject(projectId);
    if (!refreshed || !taskId) return;
    const targetCase = refreshed.cases.find((item) => item.taskId === taskId);
    if (!targetCase) return;

    let expectedName = '';
    try {
      expectedName = buildContainerCommand(targetCase).containerName.toLowerCase();
    } catch {
      return;
    }
    const matches = refreshed.containers.filter((item) => item.name.toLowerCase() === expectedName);
    if (matches.length !== 1) return;

    const matched = matches[0];
    setSelectedContainerId(matched.id);
    setBindingCompleted(false);
    setQuickPromptCopied(false);
    if (matched.state !== 'running') {
      setActionError(`已匹配容器 ${matched.name}，但容器未运行，请启动后重试。`);
      return;
    }
    if (targetCase.containerId === matched.id) {
      setBindingCompleted(true);
      await copyCurrentPrompt(true);
      return;
    }
    if (!targetCase.initialSha) {
      setActionError('已匹配容器，但题目尚未准备初始快照。');
      return;
    }
    await bindCaseToContainer(targetCase, matched.id);
  };

  const handleSaveSettings = async () => {
    if (!selectedCase) return;
    const targetProjectId = activeProjectId.current;
    const epoch = ++settingsEpoch.current;
    setSaving(true);
    setActionError('');
    setReport(null);
    try {
      const updated = await saveCaseSettings({ taskId: selectedCase.taskId, snapshotUrl, completed });
      if (targetProjectId !== activeProjectId.current || epoch !== settingsEpoch.current) return;
      replaceCase(updated);
      setNotice('题目设置已保存');
    } catch (error) {
      if (targetProjectId === activeProjectId.current && epoch === settingsEpoch.current) setActionError(errorMessage(error));
    } finally {
      if (targetProjectId === activeProjectId.current && epoch === settingsEpoch.current) setSaving(false);
    }
  };

  const handlePreflight = async () => {
    setPreflightLoading(true);
    setActionError('');
    try {
      const nextReport = await preflight(projectId);
      if (projectId !== activeProjectId.current) return;
      setReport(nextReport);
      setNotice(nextReport.issues.length ? '预检完成，请处理列出的问题或导出草稿' : '批次预检完成');
    } catch (error) {
      if (projectId === activeProjectId.current) setActionError(errorMessage(error));
    } finally {
      if (projectId === activeProjectId.current) setPreflightLoading(false);
    }
  };

  const handleExport = async (draft: boolean, exportTaskId?: string, reviewedOnly = false, taskIds?: string[]) => {
    if (taskIds && taskIds.length === 0) return;
    setActionError('');
    setExportResult(null);
    const targetProjectId = activeProjectId.current;
    const label = taskIds ? '所选题目导出' : reviewedOnly ? (exportTaskId ? '单题导出' : '一键导出已制表') : draft ? '草稿导出' : '正式导出';
    setExportBusy({ taskId: '', label, jobId: '', progress: 0, message: '正在提交后台任务' });
    try {
      const submitted = await exportCases({ projectId, submitter, submittedAt, draft, ...(reviewedOnly ? { reviewedOnly, taskId: exportTaskId } : {}), ...(taskIds ? { taskIds } : {}) });
      if (targetProjectId !== activeProjectId.current) return;
      setExportBusy({ taskId: '', label, jobId: submitted.id, progress: submitted.progress ?? 0, message: submitted.progressMessage ?? '等待执行' });
      const finished = await waitForAnnotationJob(submitted.id, (job) => {
        if (targetProjectId !== activeProjectId.current) return;
        setExportBusy({ taskId: '', label, jobId: submitted.id, progress: job.progress, message: job.progressMessage || '执行中' });
      });
      if (targetProjectId !== activeProjectId.current) return;
      const result = parseJobOutput<AnnotationExportResult>(finished, '导出完成但没有返回文件信息');
      setExportResult(result);
      setNotice(`${label}完成，共 ${result.rows} 条：${result.outputPath}`);
    } catch (error) {
      if (targetProjectId === activeProjectId.current) setActionError(errorMessage(error));
    } finally {
      if (targetProjectId === activeProjectId.current) setExportBusy(null);
    }
  };

  const handlePairwiseExport = async (draft: boolean, exportTaskId?: string) => {
    setActionError('');
    setExportResult(null);
    const targetProjectId = activeProjectId.current;
    const label = draft ? 'Pair-wise 草稿导出' : 'Pair-wise 正式导出';
    setExportBusy({ taskId: exportTaskId, label, jobId: '', progress: 0, message: '正在提交后台任务' });
    try {
      const submitted = await exportPairwise({ projectId: targetProjectId, ...(exportTaskId ? { taskId: exportTaskId } : {}), submitter, submittedAt, draft });
      setExportBusy({ taskId: exportTaskId, label, jobId: submitted.id, progress: submitted.progress ?? 0, message: submitted.progressMessage ?? '等待执行' });
      const finished = await waitForAnnotationJob(submitted.id, (job) => {
        setExportBusy({ taskId: exportTaskId, label, jobId: submitted.id, progress: job.progress, message: job.progressMessage || '执行中' });
      });
      if (targetProjectId !== activeProjectId.current) return;
      const result = parseJobOutput<AnnotationExportResult>(finished, '导出完成但没有返回文件信息');
      setExportResult(result);
      setNotice(`${label}完成，共 ${result.rows} 条：${result.outputPath}`);
    } catch (error) {
      if (targetProjectId === activeProjectId.current) setActionError(errorMessage(error));
    } finally {
      if (targetProjectId === activeProjectId.current) setExportBusy(null);
    }
  };

  const handlePairwisePreflight = async () => {
    setActionError('');
    setPreflightLoading(true);
    const targetProjectId = activeProjectId.current;
    try {
      const nextReport = await preflightPairwise(targetProjectId);
      if (targetProjectId === activeProjectId.current) {
        setPairwiseReport(nextReport);
        setNotice(nextReport.issues.length ? 'Pair-wise 预检完成，请处理列出的问题或导出草稿' : 'Pair-wise 正式导出材料齐全');
      }
    } catch (error) {
      if (targetProjectId === activeProjectId.current) setActionError(errorMessage(error));
    } finally {
      if (targetProjectId === activeProjectId.current) setPreflightLoading(false);
    }
  };

  const handleBatchPairwiseReview = async () => {
    const targetProjectId = activeProjectId.current;
    const label = '批量生成 GSB';
    setActionError('');
    setNotice('');
    setPairwiseBatchResult(null);
    setPairwiseReport(null);
    setBatchBusy({ taskId: '', label, jobId: '', progress: 0, message: '正在提交批量后台任务' });
    try {
      const submitted = await batchReviewPairwise({ projectId: targetProjectId, force: false });
      if (targetProjectId !== activeProjectId.current) return;
      setBatchBusy({ taskId: '', label, jobId: submitted.id, progress: submitted.progress ?? 0, message: submitted.progressMessage ?? '等待执行' });
      const finished = await waitForAnnotationJob(submitted.id, (job) => {
        if (targetProjectId !== activeProjectId.current) return;
        setBatchBusy({ taskId: '', label, jobId: submitted.id, progress: job.progress, message: job.progressMessage || '执行中' });
      });
      if (targetProjectId !== activeProjectId.current) return;
      const result = parseJobOutput<PairwiseBatchReviewResult>(finished, '批量 GSB 完成但没有返回汇总结果');
      setPairwiseBatchResult(result);
      setNotice(`批量 GSB 完成：新审核 ${result.reviewed} 题，复用 ${result.reused} 题，跳过 ${result.skipped} 题，失败 ${result.failed} 题`);
      await loadProject(targetProjectId);
    } catch (error) {
      if (targetProjectId === activeProjectId.current) setActionError(errorMessage(error));
    } finally {
      if (targetProjectId === activeProjectId.current) setBatchBusy(null);
    }
  };

  const handleBatchPrepare = async (taskIds: string[]) => {
    if (taskIds.length === 0) return;
    const targetProjectId = activeProjectId.current;
    const label = '批量采集并准备制表数据';
    setActionError('');
    setNotice('');
    setBatchResult(null);
    setBatchBusy({ taskId: '', label, jobId: '', progress: 0, message: '正在提交批量后台任务' });
    try {
      const submitted = await batchCaptureAndPrepareTable(taskIds);
      if (targetProjectId !== activeProjectId.current) return;
      setBatchBusy({ taskId: '', label, jobId: submitted.id, progress: submitted.progress ?? 0, message: submitted.progressMessage ?? '等待执行' });
      const finished = await waitForAnnotationJob(submitted.id, (job) => {
        if (targetProjectId !== activeProjectId.current) return;
        setBatchBusy({ taskId: '', label, jobId: submitted.id, progress: job.progress, message: job.progressMessage || '执行中' });
      });
      if (targetProjectId !== activeProjectId.current) return;
      const result = parseJobOutput<AnnotationBatchPrepareResult>(finished, '批量制表完成但没有返回汇总结果');
      setBatchResult(result);
      setNotice(`批量制表完成：新准备 ${result.prepared} 题，跳过 ${result.skipped} 题，失败 ${result.failed} 题`);
      await loadProject(targetProjectId);
    } catch (error) {
      if (targetProjectId === activeProjectId.current) {
        setActionError(errorMessage(error));
        try { await loadProject(targetProjectId); } catch { /* Preserve the batch error. */ }
      }
    } finally {
      if (targetProjectId === activeProjectId.current) setBatchBusy(null);
    }
  };

  const handleCancel = async (job: BusyAction) => {
    const targetProjectId = activeProjectId.current;
    try {
      await cancelAnnotationJob(job.jobId);
      if (targetProjectId === activeProjectId.current) setNotice('已请求取消后台任务');
    } catch (error) {
      if (targetProjectId === activeProjectId.current) setActionError(errorMessage(error));
    }
  };

  const allCasesCompleted = cases.length > 0 && cases.every((item) => item.completed);
  const formalExportReady = Boolean(report && report.issues.length === 0 && report.ready === report.rounds && allCasesCompleted);
  const pairwiseFormalExportReady = Boolean(pairwiseReport && pairwiseReport.issues.length === 0 && pairwiseReport.ready === pairwiseReport.rounds);

  return (
    <div className="min-h-full p-5 md:p-7">
      <div className="mx-auto max-w-[1500px]">
        <div className="mb-5 flex flex-wrap items-end justify-between gap-4">
          <div>
            <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.18em] text-slate-500 dark:text-slate-400">
              <Container className="h-4 w-4" /> Claude Code Docker
            </div>
            <h1 className="mt-2 text-2xl font-bold text-stone-900 dark:text-stone-50">{taskId ? (selectedCase?.mode === 'pairwise_gsb' ? 'Pair-wise GSB' : view === 'review' ? '五维 AI 复审' : '容器与轨迹') : '容器标注'}</h1>
            {!taskId && <p className="mt-1 text-sm text-stone-500 dark:text-stone-400">{`${projectName ? `${projectName} · ` : ''}${legacyCases.length ? '可批量采集并准备制表数据；已有有效评分会直接复用。' : 'Pair-wise GSB 批量审核与导出。'}`}</p>}
          </div>
          <div className="flex flex-wrap gap-2">
            {!taskId && legacyCases.length > 0 && <button className={SECONDARY_BUTTON} disabled={loading || globalBusy || Object.keys(caseBusy).length > 0} aria-expanded={showBatchSelection} onClick={() => setShowBatchSelection((open) => !open)}>{batchBusy ? <Loader2 className="h-4 w-4 animate-spin" /> : <FileSearch className="h-4 w-4" />}批量采集并准备制表数据</button>}
            {!taskId && pairwiseCases.length > 0 && <button className={PRIMARY_BUTTON} disabled={loading || globalBusy || Object.keys(caseBusy).length > 0} onClick={() => void handleBatchPairwiseReview()}><Sparkles className="h-4 w-4" />批量生成 GSB</button>}
            {!taskId && pairwiseCases.length > 0 && <button className={SECONDARY_BUTTON} disabled={loading || globalBusy || preflightLoading} onClick={() => void handlePairwisePreflight()}><FileSearch className="h-4 w-4" />Pair-wise 预检</button>}
            {!taskId && pairwiseCases.length > 0 && <button className={SECONDARY_BUTTON} disabled={loading || globalBusy} onClick={() => void handlePairwiseExport(true)}><Download className="h-4 w-4" />导出 GSB 草稿</button>}
            {!taskId && pairwiseCases.length > 0 && <button className={PRIMARY_BUTTON} disabled={loading || globalBusy || !pairwiseFormalExportReady} onClick={() => void handlePairwiseExport(false)}><Download className="h-4 w-4" />导出 GSB 正式表</button>}
            {!taskId && legacyCases.length > 0 && <button className={SECONDARY_BUTTON} disabled={loading || globalBusy} aria-expanded={showExportSelection} onClick={() => setShowExportSelection((open) => !open)}><Download className="h-4 w-4" />选择题目导出</button>}
            {!taskId && legacyCases.length > 0 && <button className={PRIMARY_BUTTON} disabled={loading || globalBusy || Object.keys(caseBusy).length > 0} onClick={() => void handleExport(true, undefined, true)}><Download className="h-4 w-4" />一键导出已制表</button>}
            {taskId && view === 'capture' && selectedCase?.mode !== 'pairwise_gsb' && (
              <button className={startupCommandCopied ? COMPLETED_BUTTON : SECONDARY_BUTTON} disabled={!startup?.value} onClick={() => void handleCopyStartupCommand()}>
                {startupCommandCopied ? <CheckCircle2 className="h-4 w-4" /> : <Clipboard className="h-4 w-4" />}
                {startupCommandCopied ? '容器命令已复制' : '复制容器命令'}
              </button>
            )}
          </div>
        </div>

        {showBatchSelection && !taskId && legacyCases.length > 0 && (
          <CrossProjectBatchSelector disabled={Boolean(batchBusy)} onSubmit={handleBatchPrepare} />
        )}

        {showExportSelection && !taskId && legacyCases.length > 0 && (
          <section aria-label="选择导出题目" className="mb-5 rounded-2xl border border-stone-200 bg-white p-4 dark:border-stone-700 dark:bg-stone-900">
            <h2 className="font-semibold text-stone-800 dark:text-stone-100">选择本项目已制表的题目</h2>
            <p className="my-2 text-xs text-stone-500">仅列出已有制表数据的题目；部分制表的题仅导出已保存轮次。所选题目合并为一个 Excel，沿用设置中的统一目录。</p>
            <div className="mb-3 flex flex-wrap gap-2">
              <button className={SECONDARY_BUTTON} disabled={globalBusy || loading || !exportableCases.length} onClick={() => setExportTaskIds(exportableCases.map((item) => item.taskId))}>全选已制表</button>
              <button className={SECONDARY_BUTTON} disabled={globalBusy || !exportTaskIds.length} onClick={() => setExportTaskIds([])}>清空选择</button>
            </div>
            <div className="max-h-64 space-y-2 overflow-y-auto">
              {exportableCases.length === 0 && <p className="py-4 text-sm text-stone-500">暂无已制表题目</p>}
              {exportableCases.map((item) => {
                const progress = getTableProgress(item);
                return <label key={item.taskId} className="flex items-start gap-3 rounded-xl border border-stone-200 p-3 text-sm dark:border-stone-700 dark:text-stone-100">
                  <input type="checkbox" aria-label={`导出 ${item.taskName} ${folderName(item.sourcePath)}`} checked={eligibleExportIds.includes(item.taskId)} disabled={globalBusy || loading} onChange={(event) => setExportTaskIds((ids) => event.target.checked ? [...ids.filter((id) => id !== item.taskId), item.taskId] : ids.filter((id) => id !== item.taskId))} />
                  <span className="min-w-0"><span className="font-semibold">{item.taskName} · {folderName(item.sourcePath)}</span><span className="ml-2 text-xs text-emerald-600">已制表 {progress.prepared}/{progress.total} 轮</span><span className="block break-all text-xs text-stone-500">{item.sourcePath}</span></span>
                </label>;
              })}
            </div>
            <button className={`${PRIMARY_BUTTON} mt-3`} disabled={loading || globalBusy || Object.keys(caseBusy).length > 0 || eligibleExportIds.length === 0} onClick={() => void handleExport(true, undefined, true, eligibleExportIds)}>导出所选题目（{eligibleExportIds.length}）</button>
          </section>
        )}

        {(loadError || actionError) && (
          <div role="alert" className="mb-4 flex items-start gap-2 rounded-2xl border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/20 dark:text-red-300">
            <AlertCircle className="mt-0.5 h-4 w-4 flex-shrink-0" /> {loadError || actionError}
          </div>
        )}
        {notice && (
          <div className="mb-4 flex items-center gap-2 break-all rounded-2xl border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-700 dark:border-emerald-900/50 dark:bg-emerald-950/20 dark:text-emerald-300">
            <CheckCircle2 className="h-4 w-4" /> {notice}
          </div>
        )}
        {batchResult && batchResult.items.some((item) => item.status !== 'prepared') && (
          <section aria-label="批量制表结果" className="mb-4 rounded-2xl border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-900/50 dark:bg-amber-950/20 dark:text-amber-200">
            <p className="font-semibold">需要留意的题目</p>
            <div className="mt-2 max-h-40 space-y-1 overflow-y-auto">
              {batchResult.items.filter((item) => item.status !== 'prepared').map((item) => <p key={item.taskId} className="break-all"><span className="font-medium">{item.taskName}</span>：{item.message}</p>)}
            </div>
          </section>
        )}
        {pairwiseBatchResult && pairwiseBatchResult.items.some((item) => item.status === 'skipped' || item.status === 'failed') && (
          <section aria-label="批量 GSB 结果" className="mb-4 rounded-2xl border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-900/50 dark:bg-amber-950/20 dark:text-amber-200">
            <p className="font-semibold">批量 GSB 待处理项</p>
            <div className="mt-2 max-h-40 space-y-1 overflow-y-auto">
              {pairwiseBatchResult.items.filter((item) => item.status === 'skipped' || item.status === 'failed').map((item) => <p key={item.taskId} className="break-all"><span className="font-medium">{item.taskName}</span>：{item.message}</p>)}
            </div>
          </section>
        )}

        <div className={taskId ? 'space-y-5' : 'grid gap-5 xl:grid-cols-[300px_minmax(0,1fr)]'}>
          {!taskId && <aside className="self-start rounded-3xl border border-stone-200 bg-white p-3 shadow-sm dark:border-stone-800 dark:bg-stone-900">
            <div className="flex items-center justify-between px-2 pb-3 pt-1">
              <h2 className="text-sm font-bold text-stone-800 dark:text-stone-100">题目进度</h2>
              <span className="text-xs text-stone-400">{cases.filter((item) => item.completed).length}/{cases.length}</span>
            </div>
            {loading ? (
              <div className="flex items-center justify-center gap-2 py-10 text-sm text-stone-400"><Loader2 className="h-4 w-4 animate-spin" />加载题目</div>
            ) : cases.length === 0 ? (
              <p className="rounded-2xl bg-stone-50 px-3 py-8 text-center text-sm text-stone-400 dark:bg-stone-800/50">当前项目暂无题目</p>
            ) : (
              <div
                aria-label="题目进度列表"
                tabIndex={0}
                className="max-h-[min(32rem,60dvh)] space-y-1.5 overflow-y-auto overscroll-contain p-1 [scrollbar-gutter:stable] xl:max-h-[calc(100dvh-15rem)]"
              >
                {cases.map((item) => {
                  const active = item.taskId === selectedTaskId;
                  const reviewed = item.rounds.filter((round) => evaluationsFor(round).length > 0).length;
                  const tableProgress = getTableProgress(item);
                  return (
                    <div key={item.taskId}>
                    <button
                      onClick={() => setSelectedTaskId(item.taskId)}
                      className={`w-full rounded-2xl px-3 py-3 text-left transition ${active ? 'bg-slate-100 ring-1 ring-slate-200 dark:bg-slate-800/70 dark:ring-slate-700' : 'hover:bg-stone-50 dark:hover:bg-stone-800/50'}`}
                    >
                      <div className="flex items-start justify-between gap-2">
                        <span className="line-clamp-2 text-sm font-semibold text-stone-800 dark:text-stone-100">{item.taskName}</span>
                        {item.completed && <CheckCircle2 className="h-4 w-4 flex-shrink-0 text-emerald-500" />}
                      </div>
                      <div className="mt-2 flex flex-wrap gap-1.5 text-[10px] font-semibold text-stone-500">
                        <span className="rounded-full bg-white px-2 py-1 dark:bg-stone-900">{item.initialSha ? '已准备' : '待准备'}</span>
                        <span className="rounded-full bg-white px-2 py-1 dark:bg-stone-900">{item.containerId ? '已绑定' : '待绑定'}</span>
                        <span className="rounded-full bg-white px-2 py-1 dark:bg-stone-900">审核 {reviewed}/{item.rounds.length}</span>
                        <TableStatusBadge progress={tableProgress} />
                      </div>
                      {caseBusy[item.taskId] && <p className="mt-2 truncate text-[11px] text-slate-500">{caseBusy[item.taskId].label} · {caseBusy[item.taskId].progress}%</p>}
                    </button>
                    {item.mode !== 'pairwise_gsb' && <button className="mb-2 mt-1 flex w-full items-center justify-center gap-1 rounded-lg py-1.5 text-xs text-slate-600 hover:bg-stone-100 disabled:opacity-40 dark:text-slate-300 dark:hover:bg-stone-800" disabled={globalBusy || Boolean(caseBusy[item.taskId]) || reviewed === 0} onClick={() => void handleExport(true, item.taskId, true)}><Download className="h-3 w-3" />导出本题 Excel</button>}
                    </div>
                  );
                })}
              </div>
            )}
          </aside>}

          <div className="min-w-0 space-y-5">
            {selectedCase ? (
              <>
                <section className="rounded-3xl border border-stone-200 bg-white p-5 shadow-sm dark:border-stone-800 dark:bg-stone-900">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div role="group" aria-label="项目信息" className="min-w-0">
                      {selectedCase.mode !== 'pairwise_gsb' && <>
                        <h2 className="text-lg font-bold text-stone-900 dark:text-stone-50">{selectedCase.taskName}</h2>
                        <p className="mt-1 break-all text-xs text-stone-400">{selectedCase.sourcePath}</p>
                      </>}
                      <div className="mt-1 flex flex-wrap items-center gap-2">
                        {selectedCase.snapshotUrl ? (
                          <a className="break-all text-xs text-indigo-500" href={selectedCase.snapshotUrl} target="_blank" rel="noreferrer">GitHub 初始环境快照：{selectedCase.snapshotUrl}</a>
                        ) : (
                          <button className="text-xs font-semibold text-indigo-500 disabled:text-stone-400" disabled={globalBusy || !selectedCase.initialSha || Boolean(selectedBusy)} onClick={() => void runCaseJob(selectedCase.taskId, '发布初始快照', () => publishSnapshot(selectedCase.taskId))}>发布 GitHub 初始快照</button>
                        )}
                      </div>
                    </div>
                    {selectedCase.mode !== 'pairwise_gsb' && <button
                      className={SECONDARY_BUTTON}
                      disabled={globalBusy || Boolean(selectedBusy) || Boolean(selectedCase.initialSha)}
                      onClick={() => void runCaseJob(selectedCase.taskId, '准备题目', () => prepareCase(selectedCase.taskId))}
                    >
                      {selectedBusy?.label === '准备题目' ? <Loader2 className="h-4 w-4 animate-spin" /> : <Archive className="h-4 w-4" />}
                      {selectedCase.initialSha ? '已准备初始快照' : '准备题目'}
                    </button>}
                  </div>
                  {view === 'capture' && selectedCase.mode !== 'pairwise_gsb' && <div role="group" aria-label="容器快捷操作" className="mt-4 flex flex-wrap items-center gap-2">
                    <select aria-label="容器" className={`${INPUT_CLASS} min-w-[260px] flex-1`} value={selectedContainerId} onChange={(event) => {
                      setSelectedContainerId(event.target.value);
                      setBindingCompleted(false);
                      setQuickPromptCopied(false);
                    }}>
                      <option value="">选择实际容器</option>
                      {containers.map((item) => <option key={item.id} value={item.id}>{item.name} · {item.state} · {item.workspacePath || '/workspace'}</option>)}
                    </select>
                    <button className={SECONDARY_BUTTON} onClick={() => void handleRefreshContainers()} disabled={loading || globalBusy || Boolean(selectedBusy)}>
                      <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />刷新容器
                    </button>
                    <button
                      className={bindingCompleted ? COMPLETED_BUTTON : PRIMARY_BUTTON}
                      disabled={globalBusy || Boolean(selectedBusy) || !selectedContainerId || !repoRelativePath.trim() || !selectedCase.initialSha}
                      onClick={() => void handleBindContainer()}
                    >
                      {selectedBusy?.label === '复制并绑定' ? <Loader2 className="h-4 w-4 animate-spin" /> : bindingCompleted && <CheckCircle2 className="h-4 w-4" />}
                      {selectedBusy?.label === '复制并绑定' ? '正在复制并绑定' : bindingCompleted ? '已绑定' : '复制并绑定'}
                    </button>
                    {taskId && <button
                      className={quickPromptCopied ? COMPLETED_BUTTON : PRIMARY_BUTTON}
                      disabled={!selectedContainerId || !promptText.trim() || !onPromptCopy}
                      onClick={() => void handleQuickPromptCopy()}
                    >
                      {quickPromptCopied ? <CheckCircle2 className="h-4 w-4" /> : <Clipboard className="h-4 w-4" />}
                      {quickPromptCopied ? '提示词已复制' : '复制提示词'}
                    </button>}
                  </div>}
                  {selectedCase.mode !== 'pairwise_gsb' && <div className="mt-4 grid gap-3 md:grid-cols-3">
                    <div className="rounded-2xl bg-stone-50 p-3 dark:bg-stone-800/50">
                      <p className="text-[11px] font-semibold text-stone-400">初始 SHA</p>
                      <p className="mt-1 break-all font-mono text-xs text-stone-700 dark:text-stone-300">{selectedCase.initialSha || '尚未建立'}</p>
                    </div>
                    <div className="rounded-2xl bg-stone-50 p-3 dark:bg-stone-800/50">
                      <p className="text-[11px] font-semibold text-stone-400">当前会话</p>
                      <p className="mt-1 break-all font-mono text-xs text-stone-700 dark:text-stone-300">{selectedCase.sessionId || '尚未采集'}</p>
                    </div>
                    <div className="rounded-2xl bg-stone-50 p-3 dark:bg-stone-800/50">
                      <p className="text-[11px] font-semibold text-stone-400">采集版本</p>
                      <p className="mt-1 text-xs text-stone-700 dark:text-stone-300">revision {selectedCase.revision} · {selectedCase.captures.length} 份</p>
                    </div>
                  </div>}
                </section>

                {selectedCase.mode !== 'pairwise_gsb' && selectedCase.rounds.length === 0 && selectedCase.captures.length === 0 && (
                  <section className="border border-stone-200 bg-white p-4 dark:border-stone-700 dark:bg-stone-900">
                    <div className="flex justify-end">
                      <button
                        className={PRIMARY_BUTTON}
                        disabled={globalBusy || Boolean(selectedBusy) || !selectedCase.initialSha}
                        onClick={() => void runCaseJob(selectedCase.taskId, '启用 Pair-wise GSB', () => enablePairwise({
                          taskId: selectedCase.taskId,
                        }))}
                      ><Sparkles className="h-4 w-4" />启用 Pair-wise GSB</button>
                    </div>
                  </section>
                )}

                {selectedCase.mode === 'pairwise_gsb' && selectedCase.pairwise ? (
                  <PairwiseWorkspace
                    annotationCase={selectedCase}
                    containers={containers}
                    disabled={globalBusy || Boolean(selectedBusy)}
                    runJob={(label, submit) => runCaseJob(selectedCase.taskId, label, submit)}
                    onCopyContainerCommand={handleCopyPairwiseStartupCommand}
                    onBindContainer={handleBindPairwiseContainer}
                    onRefreshContainers={handleRefreshPairwiseContainers}
                    onStartProject={handleStartPairwiseProject}
                    onStopProject={handleStopPairwiseProject}
                    promptCopied={pairwisePromptCopied}
                    onExport={(draft) => handlePairwiseExport(draft, selectedCase.taskId)}
                    onPreflight={handlePairwisePreflight}
                    preflightReport={pairwiseReport}
                    exportResult={exportResult}
                  />
                ) : <>

                <section className="rounded-3xl border border-stone-200 bg-white p-5 shadow-sm dark:border-stone-800 dark:bg-stone-900">
                  <div className="grid gap-3 lg:grid-cols-2 lg:items-end">
                    <label className="min-w-[280px] flex-1">
                      <span className="mb-1.5 block text-xs font-semibold text-stone-500">候选轨迹</span>
                      <select className={INPUT_CLASS} value={selectedTracePath} onChange={(event) => setSelectedTracePath(event.target.value)} disabled={!selectedCase.containerId}>
                        <option value="">{traceCandidates.length > 1 ? '请选择与本题对应的轨迹，不自动猜测' : '选择轨迹'}</option>
                        {traceCandidates.map((trace) => <option key={trace.path} value={trace.path}>{trace.sessionId || '未知会话'} · {formatBytes(trace.size)} · {trace.path}</option>)}
                      </select>
                    </label>
                    <label className="min-w-[280px] flex-1">
                      <span className="mb-1.5 block text-xs font-semibold text-stone-500">本机 JSONL 绝对路径</span>
                      <input className={INPUT_CLASS} value={selectedTracePath} onChange={(event) => setSelectedTracePath(event.target.value)} placeholder="/absolute/path/to/session.jsonl" />
                    </label>
                    <div className="flex flex-wrap gap-2 lg:col-span-2">
                    <button
                      className={PRIMARY_BUTTON}
                      disabled={globalBusy || Boolean(selectedBusy) || !selectedTracePath.trim()}
                      onClick={() => void runCaseJob(selectedCase.taskId, '采集并准备制表数据', () => captureAndPrepareTable({ taskId: selectedCase.taskId, tracePath: selectedTracePath.trim() }))}
                    ><FileSearch className="h-4 w-4" />采集并准备制表数据</button>
                    </div>
                  </div>
                  <p className="mt-3 text-xs leading-5 text-stone-500">采集并准备制表数据会保存轨迹与代码，逐轮复核提示词要求是否完成、是否引入新问题，并保存五维评分和依据。五项必须按证据独立评分，不能为了收录或凑数量压分；总分超过21的评价保留真实结果，但不进入导出。确认存在功能遗漏或 Bug 时，评分不能全满分，必须提供以“修复”开头的提示词；五维全满分时不生成修复提示词。此时不生成 Excel，单题或统一导出时才生成文件。修复建议仅供复制，不会自动执行下一轮。</p>
                  {traceCandidates.length === 0 && selectedCase.containerId && <p className="mt-2 text-xs text-amber-600 dark:text-amber-400">当前绑定未发现可选轨迹，请确认容器会话已产生记录。</p>}
                  {taskId && <p className="mt-3 text-xs text-stone-500">已采集 {selectedCase.rounds.length} 轮、{selectedCase.captures.length} 份代码与轨迹快照。评分详情在“AI复审”查看；导出按钮位于本页顶部。</p>}
                </section>

                <section className="rounded-3xl border border-stone-200 bg-white p-5 dark:border-stone-800 dark:bg-stone-900">
                  <div className="flex flex-wrap items-center gap-3">
                    <h3 className="text-base font-bold dark:text-stone-100">制表数据准备状态</h3>
                    <TableStatusBadge progress={selectedProgress!} />
                    <span className="text-xs text-stone-500">可导出 {selectedProgress!.prepared}/{selectedProgress!.total} 轮</span>
                  </div>
                  <p className="mt-2 text-xs text-stone-500">采集轨迹与代码 → 按真实轮次审核需求和五维表现 → 校验并保存数据。总分不超过21才计入可导出轮次，超限评分仍原样保留。</p>
                  {taskId && view !== 'review' && <p className="mt-2 text-xs text-stone-500">在“AI复审”中查看逐项需求核验、评分依据，以及是否需要修复。</p>}
                  {selectedBusy && <p className="mt-2 text-sm text-indigo-500">{selectedBusy.message}</p>}
                  {persistedJob && <>
                    <p className="mt-2 text-xs text-stone-500">耗时 {Math.floor((selectedProgress!.elapsed ?? 0)/60)} 分钟 · 最后活动 {new Date(persistedJob.lastActivityAt*1000).toLocaleTimeString()}</p>
                    {selectedProgress!.quiet && <p className="mt-2 text-sm text-amber-600">超过 2 分钟没有新的审核进展，可能仍在等待模型响应；尚不能判定断线。可在后台任务中取消后重试。</p>}
                    {persistedJob.error && <p className="mt-2 break-all text-sm text-red-500">{persistedJob.error}；已保存的轨迹与评分保留。</p>}
                  </>}
                  {!selectedBusy && selectedCase.rounds.some((r) => r.status==='complete') && selectedProgress &&
                    (selectedProgress.reviewed! < selectedProgress.total || Boolean(selectedProgress.missing) || Boolean(selectedProgress.stale)) &&
                    <button className={`${SECONDARY_BUTTON} mt-3`} disabled={globalBusy} onClick={() => void runCaseJob(selectedCase.taskId,'继续准备制表数据',()=>resumeTable(selectedCase.taskId))}>继续准备未完成轮次</button>}
                </section>

                {(!taskId || view === 'review') && <>
                <section className="rounded-3xl border border-stone-200 bg-white p-5 shadow-sm dark:border-stone-800 dark:bg-stone-900">
                  <div className="flex items-center justify-between gap-3">
                    <div>
                      <h3 className="text-base font-bold text-stone-900 dark:text-stone-50">真实轮次与评价</h3>
                      <p className="mt-1 text-xs text-stone-400">低分、失败和缺证据轮次都会保留；重试审核会新增历史版本。</p>
                    </div>
                    <span className="rounded-full bg-stone-100 px-3 py-1 text-xs font-semibold text-stone-600 dark:bg-stone-800 dark:text-stone-300">{selectedCase.rounds.length} 轮</span>
                  </div>
                  <div className="mt-4 space-y-4">
                    {selectedCase.rounds.length === 0 ? <p className="rounded-2xl bg-stone-50 py-8 text-center text-sm text-stone-400 dark:bg-stone-800/50">采集后将在这里显示真实有效轮次</p> : selectedCase.rounds.map((round) => (
                      <article key={`${round.sessionId}-${round.promptId}-${round.order}`} className="rounded-2xl border border-stone-200 bg-stone-50/60 p-4 dark:border-stone-700 dark:bg-stone-800/30">
                        <div className="flex flex-wrap items-start justify-between gap-3">
                          <div className="min-w-0">
                            <div className="flex flex-wrap items-center gap-2">
                              <h4 className="text-sm font-bold text-stone-800 dark:text-stone-100">第 {round.order} 轮</h4>
                              <span className="rounded-full bg-white px-2 py-0.5 text-[11px] font-semibold text-stone-500 dark:bg-stone-900">{statusLabel(round.status)}</span>
                              <span className="font-mono text-[10px] text-stone-400">{round.promptId}</span>
                            </div>
                            <p className="mt-2 whitespace-pre-wrap text-sm leading-6 text-stone-700 dark:text-stone-300">{round.prompt}</p>
                            <p className="mt-2 text-[11px] text-stone-400">来源 {round.sourceStart}-{round.sourceEnd} · cwd {round.cwd || '未记录'} · capture {round.captureId || '未记录'}</p>
                            <p className="mt-2 text-xs text-stone-500">{!latestEvaluation(round) ? '轨迹已保存，尚无审核结果；暂不能判断功能是否完成或需要修复。' : latestEvaluation(round)?.current === false ? '评价对应的规则或证据已变化，需要重新复审。' : latestEvaluation(round)?.status === 'needs_evidence' ? '已复审，待补证据，暂不可正式导出。' : (evaluationScoreTotal(latestEvaluation(round)!) ?? 0) > 21 ? '本轮评价已保存，但五维总分超过21，不进入正式导出；系统不会改低真实评分。' : '本轮评价已保存；历史版本不重复计入复审轮数。'}</p>
                            {round.reason && <p className="mt-1 text-xs text-amber-600 dark:text-amber-400">{round.reason}</p>}
                          </div>
                          <button
                            className={SECONDARY_BUTTON}
                            disabled={globalBusy || Boolean(selectedBusy) || round.status !== 'complete'}
                            onClick={() => void runCaseJob(selectedCase.taskId, evaluationsFor(round).length ? '重新审核' : '审核本轮', () => reviewRound({ taskId: selectedCase.taskId, promptId: round.promptId, force: evaluationsFor(round).length > 0 }))}
                          ><RefreshCw className="h-4 w-4" />{evaluationsFor(round).length ? '重新审核' : '审核本轮'}</button>
                        </div>
                        {evaluationsFor(round).length > 0 && (
                          <div className="mt-4 space-y-3">
                            {evaluationsFor(round).map((evaluation, index) => <EvaluationCard key={evaluation.id || `${round.promptId}-${index}`} evaluation={evaluation} index={index} />)}
                            {latestEvaluation(round)?.current !== false && latestEvaluation(round)?.nextPrompt && !isPerfectEvaluation(latestEvaluation(round)!) && (
                              <div className="rounded-2xl border border-indigo-200 bg-indigo-50 p-4 dark:border-indigo-900/50 dark:bg-indigo-950/20">
                                <div className="flex items-center justify-between gap-2">
                                  <div>
                                    <p className="text-xs font-bold text-indigo-700 dark:text-indigo-300">下一轮修复提示词（仅复制）</p>
                                    <p className="mt-0.5 text-[10px] text-indigo-500">不会自动发送，也不会在轨迹出现前计为新轮次</p>
                                  </div>
                                  <button
                                    className={SECONDARY_BUTTON}
                                    onClick={() => {
                                      const text = latestEvaluation(round)?.nextPrompt || '';
                                      void writeClipboardText(text).then(() => setNotice('下一轮建议已复制')).catch((error) => setActionError(errorMessage(error)));
                                    }}
                                  ><Clipboard className="h-4 w-4" />复制</button>
                                </div>
                                <p className="mt-3 whitespace-pre-wrap text-sm leading-6 text-indigo-900 dark:text-indigo-100">{latestEvaluation(round)?.nextPrompt}</p>
                              </div>
                            )}
                          </div>
                        )}
                      </article>
                    ))}
                  </div>
                </section>

                <section className="rounded-3xl border border-stone-200 bg-white p-5 shadow-sm dark:border-stone-800 dark:bg-stone-900">
                  <h3 className="text-base font-bold text-stone-900 dark:text-stone-50">题目登记</h3>
                  <div className="mt-4 grid gap-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-end">
                    <label>
                      <span className="mb-1.5 block text-xs font-semibold text-stone-500">初始快照 URL</span>
                      <input className={INPUT_CLASS} value={snapshotUrl} onChange={(event) => setSnapshotUrl(event.target.value)} placeholder="可留空，保存真实可访问地址" />
                    </label>
                    <label className="flex h-10 items-center gap-2 rounded-xl border border-stone-200 px-3 text-sm font-semibold text-stone-700 dark:border-stone-700 dark:text-stone-200">
                      <input type="checkbox" checked={completed} onChange={(event) => setCompleted(event.target.checked)} /> 完成本题
                    </label>
                  </div>
                  <p className="mt-2 text-xs text-stone-400">完成状态由你明确登记，不要求五项满分，也不会自动停止容器。</p>
                  <button className={`${PRIMARY_BUTTON} mt-3`} onClick={() => void handleSaveSettings()} disabled={globalBusy || saving || Boolean(selectedBusy)}>
                    {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}保存题目设置
                  </button>
                </section>
                </>}
                </>}
              </>
            ) : !loading && (
              <div className="rounded-3xl border border-dashed border-stone-300 bg-white py-20 text-center text-sm text-stone-400 dark:border-stone-700 dark:bg-stone-900">{taskId ? '当前题目暂无标注记录，请先确认题目已导入当前项目' : '选择一项题目开始标注'}</div>
            )}

            {!taskId && selectedCase?.mode !== 'pairwise_gsb' && <section className="rounded-3xl border border-stone-200 bg-white p-5 shadow-sm dark:border-stone-800 dark:bg-stone-900">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <h3 className="text-base font-bold text-stone-900 dark:text-stone-50">批次预检与统一导出</h3>
                  <p className="mt-1 text-xs text-stone-400">导出只汇总已保存且五维总分不超过21的评价，不会自动审核或改分。请先完成各轮审核；草稿允许评分留空。</p>
                </div>
                <button className={SECONDARY_BUTTON} onClick={() => void handlePreflight()} disabled={preflightLoading || globalBusy || Object.keys(caseBusy).length > 0 || saving}>
                  {preflightLoading ? <Loader2 className="h-4 w-4 animate-spin" /> : <FileSearch className="h-4 w-4" />}批次预检
                </button>
              </div>
              {report && (
                <div className="mt-4 rounded-2xl bg-stone-50 p-4 dark:bg-stone-800/50">
                  <div className="grid grid-cols-2 gap-3 text-center md:grid-cols-4">
                    <div><p className="text-xl font-bold text-stone-800 dark:text-stone-100">{report.tasks}</p><p className="text-[11px] text-stone-400">题目</p></div>
                    <div><p className="text-xl font-bold text-stone-800 dark:text-stone-100">{report.rounds}</p><p className="text-[11px] text-stone-400">真实轮次</p></div>
                    <div><p className="text-xl font-bold text-stone-800 dark:text-stone-100">{report.ready}</p><p className="text-[11px] text-stone-400">可导出评价</p></div>
                    <div><p className="text-xl font-bold text-stone-800 dark:text-stone-100">{report.notCollected ?? 0}</p><p className="text-[11px] text-stone-400">超过21不收录</p></div>
                  </div>
                  {report.issues.length > 0 && <ul className="mt-3 space-y-1 border-t border-stone-200 pt-3 text-xs text-amber-700 dark:border-stone-700 dark:text-amber-300">{report.issues.map((issue) => <li key={issue}>• {issue}</li>)}</ul>}
                </div>
              )}
              <div className="mt-4 grid gap-3 md:grid-cols-2">
                <label>
                  <span className="mb-1.5 block text-xs font-semibold text-stone-500">提交人</span>
                  <input className={INPUT_CLASS} value={submitter} onChange={(event) => setSubmitter(event.target.value)} placeholder="按真实提交身份填写，可留空" />
                </label>
                <label>
                  <span className="mb-1.5 block text-xs font-semibold text-stone-500">实际提交时间（可选）</span>
                  <input type="datetime-local" className={INPUT_CLASS} value={submittedAt} onChange={(event) => setSubmittedAt(event.target.value)} />
                </label>
              </div>
              <div className="mt-3 flex flex-wrap gap-2">
                {formalExportReady ? (
                  <button className={PRIMARY_BUTTON} disabled={globalBusy} onClick={() => void handleExport(false)}><Download className="h-4 w-4" />正式导出</button>
                ) : (
                  <button className={SECONDARY_BUTTON} disabled={globalBusy || Object.keys(caseBusy).length > 0 || saving || cases.length === 0} onClick={() => void handleExport(true)}><Download className="h-4 w-4" />导出待补草稿</button>
                )}
                {!report && <span className="self-center text-xs text-amber-600 dark:text-amber-400">正式导出前请先执行批次预检</span>}
              </div>
              {exportResult && (
                <div className="mt-4 rounded-2xl border border-emerald-200 bg-emerald-50 p-4 text-xs text-emerald-800 dark:border-emerald-900/50 dark:bg-emerald-950/20 dark:text-emerald-200">
                  <p className="font-bold">已导出 {exportResult.rows} 行</p>
                  <p className="mt-1 break-all">Excel：{exportResult.outputPath}</p>
                  <p className="mt-1 break-all">检查报告：{exportResult.reportPath}</p>
                  {exportResult.issues.length > 0 && <ul className="mt-2 space-y-1">{exportResult.issues.map((issue) => <li key={issue}>• {issue}</li>)}</ul>}
                </div>
              )}
            </section>}
          </div>
        </div>
      </div>

      {visibleBusy && (
        <div className="fixed bottom-5 right-5 z-40 w-[min(360px,calc(100vw-40px))] rounded-2xl border border-stone-200 bg-white p-4 shadow-xl dark:border-stone-700 dark:bg-stone-900">
          <div className="flex items-center justify-between gap-3">
            <div className="min-w-0">
              <p className="text-sm font-bold text-stone-800 dark:text-stone-100">{visibleBusy.label}</p>
              <p className="mt-1 truncate text-xs text-stone-400">{visibleBusy.message}</p>
            </div>
            <button className={SECONDARY_BUTTON} disabled={!visibleBusy.jobId} onClick={() => void handleCancel(visibleBusy)}><Square className="h-3 w-3" />取消</button>
          </div>
          <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-stone-100 dark:bg-stone-800"><div className="h-full bg-slate-600 transition-all" style={{ width: `${Math.max(2, visibleBusy.progress)}%` }} /></div>
        </div>
      )}
    </div>
  );
}

export default function Annotation() {
  const activeProject = useAppStore((state) => state.activeProject);
  if (!activeProject?.id) {
    return (
      <div className="flex min-h-full items-center justify-center p-8">
        <div className="max-w-md rounded-3xl border border-dashed border-stone-300 bg-white px-8 py-14 text-center dark:border-stone-700 dark:bg-stone-900">
          <Container className="mx-auto h-8 w-8 text-stone-300" />
          <h1 className="mt-4 text-lg font-bold text-stone-800 dark:text-stone-100">容器标注</h1>
          <p className="mt-2 text-sm text-stone-400">请先在左侧选择一个项目。</p>
        </div>
      </div>
    );
  }
  return <AnnotationWorkspace projectId={activeProject.id} projectName={activeProject.name} />;
}
