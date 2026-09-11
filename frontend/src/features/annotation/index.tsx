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
  Square,
} from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  bindContainer,
  cancelAnnotationJob,
  captureCase,
  exportCases,
  listCases,
  listContainers,
  listTraces,
  preflight,
  prepareCase,
  reviewRound,
  saveCaseSettings,
  type AnnotationCase,
  type AnnotationContainer,
  type AnnotationEvaluation,
  type AnnotationExportResult,
  type AnnotationPreflightReport,
  type AnnotationRound,
  type TraceCandidate,
} from '../../api/annotation';
import type { BackgroundJob } from '../../api/job';
import { useAppStore } from '../../store';
import { waitForAnnotationJob } from './job';

const INPUT_CLASS = 'w-full rounded-xl border border-stone-200 bg-white px-3 py-2 text-sm text-stone-800 outline-none transition focus:border-slate-400 focus:ring-2 focus:ring-slate-200 dark:border-stone-700 dark:bg-[#171B22] dark:text-stone-100 dark:focus:border-slate-500 dark:focus:ring-slate-800';
const PRIMARY_BUTTON = 'inline-flex items-center justify-center gap-2 rounded-xl bg-slate-800 px-3.5 py-2 text-sm font-semibold text-white transition hover:bg-slate-700 disabled:cursor-not-allowed disabled:opacity-40 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white';
const SECONDARY_BUTTON = 'inline-flex items-center justify-center gap-2 rounded-xl border border-stone-200 bg-white px-3.5 py-2 text-sm font-semibold text-stone-700 transition hover:bg-stone-50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-stone-700 dark:bg-stone-800 dark:text-stone-200 dark:hover:bg-stone-700';

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
};

function folderName(path: string) {
  const normalized = path.replace(/[\\/]+$/, '');
  return normalized.split(/[\\/]/).pop() || 'repository';
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

function statusLabel(status: string) {
  return ({ complete: '完整', pending: '未完成', conflict: '有冲突', excluded: '已排除' } as Record<string, string>)[status] ?? status;
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
          {evaluation.status || '未知状态'}
        </span>
      </div>
      <ScoreGrid evaluation={evaluation} />

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
    </div>
  );
}

export function AnnotationWorkspace({ projectId, projectName }: AnnotationWorkspaceProps) {
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
  const [preflightLoading, setPreflightLoading] = useState(false);
  const [submitter, setSubmitter] = useState('');
  const [submittedAt, setSubmittedAt] = useState('');
  const [exportResult, setExportResult] = useState<AnnotationExportResult | null>(null);
  const [exportBusy, setExportBusy] = useState<BusyAction | null>(null);
  const projectEpoch = useRef(0);
  const settingsEpoch = useRef(0);
  const activeProjectId = useRef(projectId);

  const selectedCase = useMemo(
    () => cases.find((item) => item.taskId === selectedTaskId) ?? null,
    [cases, selectedTaskId],
  );
  const selectedBusy = selectedCase ? caseBusy[selectedCase.taskId] ?? null : null;
  const visibleBusy = exportBusy ?? selectedBusy ?? Object.values(caseBusy)[0] ?? null;

  const loadProject = useCallback(async (targetProjectId: string) => {
    const epoch = ++projectEpoch.current;
    setLoading(true);
    setLoadError('');
    try {
      const [caseResult, containerResult] = await Promise.allSettled([
        listCases(targetProjectId),
        listContainers(),
      ]);
      if (epoch !== projectEpoch.current || targetProjectId !== activeProjectId.current) return;
      const nextCases = caseResult.status === 'fulfilled' ? caseResult.value : [];
      const nextContainers = containerResult.status === 'fulfilled' ? containerResult.value : [];
      setCases(nextCases);
      setContainers(nextContainers);
      setSelectedTaskId((current) => nextCases.some((item) => item.taskId === current) ? current : (nextCases[0]?.taskId ?? ''));
      const errors = [
        caseResult.status === 'rejected' ? `题目加载失败：${errorMessage(caseResult.reason)}` : '',
        containerResult.status === 'rejected' ? `容器列表加载失败：${errorMessage(containerResult.reason)}` : '',
      ].filter(Boolean);
      setLoadError(errors.join('；'));
    } catch (error) {
      if (epoch !== projectEpoch.current || targetProjectId !== activeProjectId.current) return;
      setLoadError(errorMessage(error));
      setCases([]);
    } finally {
      if (epoch === projectEpoch.current && targetProjectId === activeProjectId.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    activeProjectId.current = projectId;
    setReport(null);
    setExportResult(null);
    setCaseBusy({});
    setExportBusy(null);
    setSaving(false);
    setPreflightLoading(false);
    settingsEpoch.current += 1;
    setSelectedTaskId('');
    void loadProject(projectId);
    return () => { projectEpoch.current += 1; };
  }, [loadProject, projectId]);

  useEffect(() => {
    if (!selectedCase) return;
    setSelectedContainerId(selectedCase.containerId || '');
    setRepoRelativePath(selectedCase.repoRelativePath || folderName(selectedCase.sourcePath));
    setSnapshotUrl(selectedCase.snapshotUrl || '');
    setCompleted(selectedCase.completed);
    setSelectedTracePath(selectedCase.tracePath || '');
  }, [selectedCase?.taskId, selectedCase?.revision]);

  useEffect(() => {
    setNotice('');
    setActionError('');
  }, [selectedCase?.taskId]);

  useEffect(() => {
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
  }, [selectedCase?.containerId, selectedCase?.taskId, selectedCase?.tracePath, selectedCase?.revision]);

  const replaceCase = useCallback((updated: AnnotationCase) => {
    if (updated.projectId && updated.projectId !== activeProjectId.current) return;
    setCases((current) => current.map((item) => item.taskId === updated.taskId ? updated : item));
    setReport(null);
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
    updateCaseBusy(taskId, { taskId, label, jobId: '', progress: 0, message: '正在提交后台任务' });
    try {
      const submitted = await submit();
      if (targetProjectId !== activeProjectId.current) return;
      updateCaseBusy(taskId, { taskId, label, jobId: submitted.id, progress: submitted.progress ?? 0, message: submitted.progressMessage ?? '等待执行' });
      const finished = await waitForAnnotationJob(submitted.id, (job) => {
        if (targetProjectId !== activeProjectId.current) return;
        updateCaseBusy(taskId, { taskId, label, jobId: submitted.id, progress: job.progress, message: job.progressMessage || '执行中' });
      });
      if (targetProjectId !== activeProjectId.current) return;
      const updated = parseJobOutput<AnnotationCase>(finished, `${label}完成但没有返回题目结果`);
      replaceCase(updated);
      setNotice(`${label}已完成`);
    } catch (error) {
      if (targetProjectId === activeProjectId.current) setActionError(errorMessage(error));
    } finally {
      if (targetProjectId === activeProjectId.current) updateCaseBusy(taskId, null);
    }
  }, [replaceCase, updateCaseBusy]);

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

  const handleExport = async (draft: boolean) => {
    setActionError('');
    setExportResult(null);
    const targetProjectId = activeProjectId.current;
    const label = draft ? '草稿导出' : '正式导出';
    setExportBusy({ taskId: '', label, jobId: '', progress: 0, message: '正在提交后台任务' });
    try {
      const submitted = await exportCases({ projectId, submitter, submittedAt, draft });
      if (targetProjectId !== activeProjectId.current) return;
      setExportBusy({ taskId: '', label, jobId: submitted.id, progress: submitted.progress ?? 0, message: submitted.progressMessage ?? '等待执行' });
      const finished = await waitForAnnotationJob(submitted.id, (job) => {
        if (targetProjectId !== activeProjectId.current) return;
        setExportBusy({ taskId: '', label, jobId: submitted.id, progress: job.progress, message: job.progressMessage || '执行中' });
      });
      if (targetProjectId !== activeProjectId.current) return;
      setExportResult(parseJobOutput<AnnotationExportResult>(finished, '导出完成但没有返回文件信息'));
      setNotice(draft ? '草稿材料包已导出' : '本批材料已正式导出');
    } catch (error) {
      if (targetProjectId === activeProjectId.current) setActionError(errorMessage(error));
    } finally {
      if (targetProjectId === activeProjectId.current) setExportBusy(null);
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

  return (
    <div className="min-h-full p-5 md:p-7">
      <div className="mx-auto max-w-[1500px]">
        <div className="mb-5 flex flex-wrap items-end justify-between gap-4">
          <div>
            <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.18em] text-slate-500 dark:text-slate-400">
              <Container className="h-4 w-4" /> Claude Code Docker
            </div>
            <h1 className="mt-2 text-2xl font-bold text-stone-900 dark:text-stone-50">容器标注</h1>
            <p className="mt-1 text-sm text-stone-500 dark:text-stone-400">
              {projectName ? `${projectName} · ` : ''}逐题绑定容器、采集真实轨迹并保存五维评价，完成后统一导出。
            </p>
          </div>
          <button className={SECONDARY_BUTTON} onClick={() => void loadProject(projectId)} disabled={loading}>
            <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} /> 刷新
          </button>
        </div>

        {(loadError || actionError) && (
          <div role="alert" className="mb-4 flex items-start gap-2 rounded-2xl border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/20 dark:text-red-300">
            <AlertCircle className="mt-0.5 h-4 w-4 flex-shrink-0" /> {loadError || actionError}
          </div>
        )}
        {notice && (
          <div className="mb-4 flex items-center gap-2 rounded-2xl border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-700 dark:border-emerald-900/50 dark:bg-emerald-950/20 dark:text-emerald-300">
            <CheckCircle2 className="h-4 w-4" /> {notice}
          </div>
        )}

        <div className="grid gap-5 xl:grid-cols-[300px_minmax(0,1fr)]">
          <aside className="self-start rounded-3xl border border-stone-200 bg-white p-3 shadow-sm dark:border-stone-800 dark:bg-stone-900">
            <div className="flex items-center justify-between px-2 pb-3 pt-1">
              <h2 className="text-sm font-bold text-stone-800 dark:text-stone-100">题目进度</h2>
              <span className="text-xs text-stone-400">{cases.filter((item) => item.completed).length}/{cases.length}</span>
            </div>
            {loading ? (
              <div className="flex items-center justify-center gap-2 py-10 text-sm text-stone-400"><Loader2 className="h-4 w-4 animate-spin" />加载题目</div>
            ) : cases.length === 0 ? (
              <p className="rounded-2xl bg-stone-50 px-3 py-8 text-center text-sm text-stone-400 dark:bg-stone-800/50">当前项目暂无题目</p>
            ) : (
              <div className="space-y-1.5">
                {cases.map((item) => {
                  const active = item.taskId === selectedTaskId;
                  const reviewed = item.rounds.filter((round) => evaluationsFor(round).length > 0).length;
                  return (
                    <button
                      key={item.taskId}
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
                      </div>
                      {caseBusy[item.taskId] && <p className="mt-2 truncate text-[11px] text-slate-500">{caseBusy[item.taskId].label} · {caseBusy[item.taskId].progress}%</p>}
                    </button>
                  );
                })}
              </div>
            )}
          </aside>

          <div className="min-w-0 space-y-5">
            {selectedCase ? (
              <>
                <section className="rounded-3xl border border-stone-200 bg-white p-5 shadow-sm dark:border-stone-800 dark:bg-stone-900">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div>
                      <h2 className="text-lg font-bold text-stone-900 dark:text-stone-50">{selectedCase.taskName}</h2>
                      <p className="mt-1 break-all text-xs text-stone-400">{selectedCase.sourcePath}</p>
                    </div>
                    <button
                      className={SECONDARY_BUTTON}
                      disabled={Boolean(selectedBusy) || Boolean(selectedCase.initialSha)}
                      onClick={() => void runCaseJob(selectedCase.taskId, '准备题目', () => prepareCase(selectedCase.taskId))}
                    >
                      {selectedBusy?.label === '准备题目' ? <Loader2 className="h-4 w-4 animate-spin" /> : <Archive className="h-4 w-4" />}
                      {selectedCase.initialSha ? '已准备初始快照' : '准备题目'}
                    </button>
                  </div>
                  <div className="mt-4 grid gap-3 md:grid-cols-3">
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
                  </div>
                </section>

                <section className="rounded-3xl border border-stone-200 bg-white p-5 shadow-sm dark:border-stone-800 dark:bg-stone-900">
                  <h3 className="text-base font-bold text-stone-900 dark:text-stone-50">绑定执行容器</h3>
                  <div className="mt-3 rounded-2xl border border-blue-200 bg-blue-50 p-3 text-xs leading-5 text-blue-800 dark:border-blue-900/50 dark:bg-blue-950/20 dark:text-blue-200">
                    新容器启动后 <span className="font-mono">/workspace</span> 必须为空；请在首次输入 Prompt 前完成准备、复制和绑定。关联已有仓库会保留容器内结果，但若首轮前快照缺失，系统无法补建真实初始状态。
                  </div>
                  <div className="mt-4 grid gap-3 lg:grid-cols-2">
                    <label className="block">
                      <span className="mb-1.5 block text-xs font-semibold text-stone-500">容器</span>
                      <select className={INPUT_CLASS} value={selectedContainerId} onChange={(event) => setSelectedContainerId(event.target.value)}>
                        <option value="">选择实际容器</option>
                        {containers.map((item) => <option key={item.id} value={item.id}>{item.name} · {item.state} · {item.workspacePath || '/workspace'}</option>)}
                      </select>
                    </label>
                    <label className="block">
                      <span className="mb-1.5 block text-xs font-semibold text-stone-500">仓库相对路径</span>
                      <input className={INPUT_CLASS} value={repoRelativePath} onChange={(event) => setRepoRelativePath(event.target.value)} placeholder="repository" />
                    </label>
                  </div>
                  <div className="mt-3 flex flex-wrap gap-2">
                    <button
                      className={PRIMARY_BUTTON}
                      disabled={Boolean(selectedBusy) || !selectedContainerId || !repoRelativePath.trim() || !selectedCase.initialSha}
                      onClick={() => void runCaseJob(selectedCase.taskId, '复制并绑定', () => bindContainer({ taskId: selectedCase.taskId, containerId: selectedContainerId, repoRelativePath: repoRelativePath.trim(), copyRepository: true }))}
                    >复制并绑定</button>
                    <button
                      className={SECONDARY_BUTTON}
                      disabled={Boolean(selectedBusy) || !selectedContainerId || !repoRelativePath.trim()}
                      onClick={() => void runCaseJob(selectedCase.taskId, '关联已有仓库', () => bindContainer({ taskId: selectedCase.taskId, containerId: selectedContainerId, repoRelativePath: repoRelativePath.trim(), copyRepository: false }))}
                    >关联已有仓库</button>
                  </div>
                </section>

                <section className="rounded-3xl border border-stone-200 bg-white p-5 shadow-sm dark:border-stone-800 dark:bg-stone-900">
                  <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] lg:items-end">
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
                    <button
                      className={PRIMARY_BUTTON}
                      disabled={Boolean(selectedBusy) || !selectedTracePath.trim()}
                      onClick={() => void runCaseJob(selectedCase.taskId, '采集轨迹', () => captureCase({ taskId: selectedCase.taskId, tracePath: selectedTracePath.trim() }))}
                    ><FileSearch className="h-4 w-4" />采集轨迹</button>
                  </div>
                  {traceCandidates.length === 0 && selectedCase.containerId && <p className="mt-2 text-xs text-amber-600 dark:text-amber-400">当前绑定未发现可选轨迹，请确认容器会话已产生记录。</p>}
                </section>

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
                            {round.reason && <p className="mt-1 text-xs text-amber-600 dark:text-amber-400">{round.reason}</p>}
                          </div>
                          <button
                            className={SECONDARY_BUTTON}
                            disabled={Boolean(selectedBusy) || round.status !== 'complete'}
                            onClick={() => void runCaseJob(selectedCase.taskId, evaluationsFor(round).length ? '重新审核' : '审核本轮', () => reviewRound({ taskId: selectedCase.taskId, promptId: round.promptId, force: evaluationsFor(round).length > 0 }))}
                          ><RefreshCw className="h-4 w-4" />{evaluationsFor(round).length ? '重新审核' : '审核本轮'}</button>
                        </div>
                        {evaluationsFor(round).length > 0 && (
                          <div className="mt-4 space-y-3">
                            {evaluationsFor(round).map((evaluation, index) => <EvaluationCard key={evaluation.id || `${round.promptId}-${index}`} evaluation={evaluation} index={index} />)}
                            {evaluationsFor(round).at(-1)?.nextPrompt && (
                              <div className="rounded-2xl border border-indigo-200 bg-indigo-50 p-4 dark:border-indigo-900/50 dark:bg-indigo-950/20">
                                <div className="flex items-center justify-between gap-2">
                                  <div>
                                    <p className="text-xs font-bold text-indigo-700 dark:text-indigo-300">下一轮建议（仅复制）</p>
                                    <p className="mt-0.5 text-[10px] text-indigo-500">不会自动发送，也不会在轨迹出现前计为新轮次</p>
                                  </div>
                                  <button
                                    className={SECONDARY_BUTTON}
                                    onClick={() => {
                                      const text = evaluationsFor(round).at(-1)?.nextPrompt || '';
                                      void navigator.clipboard.writeText(text).then(() => setNotice('下一轮建议已复制')).catch((error) => setActionError(errorMessage(error)));
                                    }}
                                  ><Clipboard className="h-4 w-4" />复制</button>
                                </div>
                                <p className="mt-3 whitespace-pre-wrap text-sm leading-6 text-indigo-900 dark:text-indigo-100">{evaluationsFor(round).at(-1)?.nextPrompt}</p>
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
                  <button className={`${PRIMARY_BUTTON} mt-3`} onClick={() => void handleSaveSettings()} disabled={saving || Boolean(selectedBusy)}>
                    {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}保存题目设置
                  </button>
                </section>
              </>
            ) : !loading && (
              <div className="rounded-3xl border border-dashed border-stone-300 bg-white py-20 text-center text-sm text-stone-400 dark:border-stone-700 dark:bg-stone-900">选择一项题目开始标注</div>
            )}

            <section className="rounded-3xl border border-stone-200 bg-white p-5 shadow-sm dark:border-stone-800 dark:bg-stone-900">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <h3 className="text-base font-bold text-stone-900 dark:text-stone-50">批次预检与统一导出</h3>
                  <p className="mt-1 text-xs text-stone-400">正式导出前必须重新预检。材料缺失时可导出完整草稿，问题不会被静默忽略。</p>
                </div>
                <button className={SECONDARY_BUTTON} onClick={() => void handlePreflight()} disabled={preflightLoading || Boolean(exportBusy) || Object.keys(caseBusy).length > 0 || saving}>
                  {preflightLoading ? <Loader2 className="h-4 w-4 animate-spin" /> : <FileSearch className="h-4 w-4" />}批次预检
                </button>
              </div>
              {report && (
                <div className="mt-4 rounded-2xl bg-stone-50 p-4 dark:bg-stone-800/50">
                  <div className="grid grid-cols-3 gap-3 text-center">
                    <div><p className="text-xl font-bold text-stone-800 dark:text-stone-100">{report.tasks}</p><p className="text-[11px] text-stone-400">题目</p></div>
                    <div><p className="text-xl font-bold text-stone-800 dark:text-stone-100">{report.rounds}</p><p className="text-[11px] text-stone-400">真实轮次</p></div>
                    <div><p className="text-xl font-bold text-stone-800 dark:text-stone-100">{report.ready}</p><p className="text-[11px] text-stone-400">可导出评价</p></div>
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
                  <button className={PRIMARY_BUTTON} disabled={Boolean(exportBusy)} onClick={() => void handleExport(false)}><Download className="h-4 w-4" />正式导出</button>
                ) : (
                  <button className={SECONDARY_BUTTON} disabled={Boolean(exportBusy) || Object.keys(caseBusy).length > 0 || saving || cases.length === 0} onClick={() => void handleExport(true)}><Download className="h-4 w-4" />草稿导出</button>
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
            </section>
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
