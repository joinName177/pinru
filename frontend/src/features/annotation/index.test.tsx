import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { AnnotationCase } from '../../api/annotation';
import { AnnotationWorkspace } from './index';

const api = vi.hoisted(() => ({
  getConfig: vi.fn(),
  bindContainer: vi.fn(),
  batchCaptureAndPrepareTable: vi.fn(),
  cancelAnnotationJob: vi.fn(),
  captureAndPrepareTable: vi.fn(),
  exportCases: vi.fn(),
  getAnnotationJob: vi.fn(),
  listCases: vi.fn(),
  listContainers: vi.fn(),
  listTraces: vi.fn(),
  preflight: vi.fn(),
  prepareCase: vi.fn(),
  reviewRound: vi.fn(),
  saveCaseSettings: vi.fn(),
}));

vi.mock('../../api/config', async () => ({
  ...await vi.importActual<typeof import('../../api/config')>('../../api/config'),
  getConfig: api.getConfig,
}));

vi.mock('../../api/annotation', async () => {
  const actual = await vi.importActual<typeof import('../../api/annotation')>('../../api/annotation');
  return { ...actual, ...api };
});

function makeCase(overrides: Partial<AnnotationCase> = {}): AnnotationCase {
  return {
    taskId: 'task-1',
    projectId: 'project-1',
    taskName: '低分也保留的任务',
    sourcePath: '/source/repo-one',
    initialSha: 'abc123',
    snapshotUrl: '',
    containerId: 'container-1',
    containerName: 'claude-one',
    workspacePath: '/workspace',
    repoRelativePath: 'repo-one',
    sessionId: 'session-1',
    tracePath: '/trace/session-1.jsonl',
    completed: false,
    rounds: [
      {
        promptId: 'prompt-1',
        sessionId: 'session-1',
        prompt: '修复真实问题',
        order: 1,
        status: 'complete',
        reason: '',
        evidenceHash: 'evidence-1',
        sourceStart: 10,
        sourceEnd: 40,
        version: 'v1',
        cwd: '/workspace/repo-one',
        captureId: 'capture-1',
        evaluations: [
          {
            id: 'evaluation-1',
            createdAt: 1,
            skillHash: 'skill-1',
            model: 'reviewer',
            evidenceHash: 'evidence-1',
            status: 'complete',
            scores: [1, 2, null, 4, 5],
            descriptions: ['理解不足', '规划有遗漏', '验证证据缺失', '实现基本正确', '交付清楚'],
            taskType: 'bugfix',
            difficulty: 'medium',
            language: 'TypeScript',
            environment: 'docker',
            harnessVersion: '1',
            os: 'linux',
            evidence: ['轨迹 10-40', 'capture-1'],
            missing: ['独立验证输出'],
            issues: [{ description: '边界未覆盖', evidence: '轨迹 31', kind: 'verification' }],
            nextPrompt: '补充边界测试并记录结果',
            nextPromptType: 'verification',
          },
        ],
      },
    ],
    captures: [],
    revision: 1,
    updatedAt: 1,
    ...overrides,
  };
}

describe('AnnotationWorkspace', () => {
  beforeEach(() => {
    Object.values(api).forEach((mock) => mock.mockReset());
    api.getConfig.mockResolvedValue('');
    api.listCases.mockResolvedValue([makeCase()]);
    api.listContainers.mockResolvedValue([]);
    api.listTraces.mockResolvedValue([]);
  });

  it('restores running state and disables duplicate reviews after reopening details', async () => {
    const c=makeCase({preparation:{jobId:'running-job',status:'running',progress:40,message:'第 1 轮：等待模型响应',error:'',startedAt:1,finishedAt:0,lastActivityAt:1}});
    c.rounds[0].evaluations=[];
    api.listCases.mockResolvedValue([c]);
    render(<AnnotationWorkspace projectId="project-1" taskId="task-1" view="review" />);
    expect(await screen.findByText('制表中 · 已复审 0/1 轮')).toBeInTheDocument();
    expect(screen.getByRole('button',{name:'审核本轮'})).toBeDisabled();
    expect(screen.getByText(/超过 2 分钟没有新的审核进展/)).toBeInTheDocument();
  });

  it('shows persisted failure and offers resume without requiring the container', async () => {
    const c=makeCase({containerId:'',preparation:{jobId:'failed-job',status:'error',progress:15,message:'第 1 轮',error:'审核执行超时',startedAt:1,finishedAt:100,lastActivityAt:50}});
    c.rounds[0].evaluations=[];
    api.listCases.mockResolvedValue([c]);
    render(<AnnotationWorkspace projectId="project-1" taskId="task-1" view="review" />);
    expect(await screen.findByText('制表失败 · 已复审 0/1 轮')).toBeInTheDocument();
    expect(screen.getByRole('button',{name:'继续准备未完成轮次'})).toBeEnabled();
    expect(screen.getByText(/审核执行超时；已保存的轨迹与评分保留/)).toBeInTheDocument();
  });

  it('explains completed functionality with process deductions without demanding repair', async () => {
    const c=makeCase();
    const e=c.rounds[0].evaluations![0];
    e.status='ready'; e.scores=[5,5,4,5,4]; e.issues=[{kind:'process',description:'重复读取',evidence:'原轨迹两次读取同一文件'}];e.nextPrompt='';e.missing=[];
    e.requirementChecks=[{requirement:'支持导出',status:'completed',evidence:'评价助手执行导出测试通过'}];
    api.listCases.mockResolvedValue([c]);
    render(<AnnotationWorkspace projectId="project-1" taskId="task-1" view="review" />);
    expect(await screen.findByText('已完成 · 支持导出')).toBeInTheDocument();
    expect(screen.getByText(/本轮审核未发现待修复的代码问题/)).toBeInTheDocument();
    expect(screen.queryByText('下一轮修复提示词（仅复制）')).not.toBeInTheDocument();
  });

  it('prepares table data directly from the capture panel without another round', async () => {
    api.captureAndPrepareTable.mockResolvedValue({ id: 'table-job', status: 'pending' });
    api.getAnnotationJob.mockResolvedValue({ id: 'table-job', status: 'done', outputPayload: JSON.stringify(makeCase()) });
    render(<AnnotationWorkspace projectId="project-1" taskId="task-1" />);
    const button = await screen.findByRole('button', { name: '采集并准备制表数据' });
    fireEvent.change(screen.getByLabelText('本机 JSONL 绝对路径'), { target: { value: '/tmp/current.jsonl' } });
    fireEvent.click(button);
    await waitFor(() => expect(api.captureAndPrepareTable).toHaveBeenCalledWith({ taskId: 'task-1', tracePath: '/tmp/current.jsonl' }));
    expect(await screen.findByText('采集并准备制表数据已完成')).toBeInTheDocument();
    expect(api.reviewRound).not.toHaveBeenCalled();
    expect(api.saveCaseSettings).not.toHaveBeenCalled();
  });

  it('offers project-wide export inside task details without limiting it to this task', async () => {
    api.exportCases.mockResolvedValue({ id: 'export-job', status: 'pending' });
    api.getAnnotationJob.mockResolvedValue({ id: 'export-job', status: 'done', outputPayload: JSON.stringify({ outputPath: '/exports/submission.xlsx', rows: 1, issues: [] }) });
    render(<AnnotationWorkspace projectId="project-1" taskId="task-1" />);
    const button = await screen.findByRole('button', { name: '导出全项目已制表' });
    await waitFor(() => expect(button).toBeEnabled());
    fireEvent.click(button);
    await waitFor(() => expect(api.exportCases).toHaveBeenCalledWith(expect.objectContaining({ projectId: 'project-1', reviewedOnly: true, taskId: undefined })));
    await screen.findByText(/一键导出已制表完成/);
  });

  it('exports only checked prepared tasks and resets the selection between projects', async () => {
    const first = makeCase({ taskName: '第一题' });
    first.rounds[0].evaluations![0].status = 'ready';
    const second = makeCase({ taskId: 'task-2', taskName: '第二题' });
    second.rounds[0].evaluations![0].status = 'ready';
    api.listCases.mockResolvedValue([first, second, makeCase({ taskId: 'task-3', taskName: '未制表题', rounds: [] })]);
    api.exportCases.mockResolvedValue({ id: 'selected-export', status: 'pending' });
    api.getAnnotationJob.mockResolvedValue({ id: 'selected-export', status: 'done', outputPayload: JSON.stringify({ outputPath: '/exports/selected.xlsx', rows: 1, issues: [] }) });
    const { rerender } = render(<AnnotationWorkspace projectId="project-1" taskId="task-1" />);
    const open = await screen.findByRole('button', { name: '选择题目导出' });
    await waitFor(() => expect(open).toBeEnabled());
    fireEvent.click(open);
    expect(screen.getByRole('button', { name: '导出所选题目（0）' })).toBeDisabled();
    expect(screen.queryByRole('checkbox', { name: /导出 未制表题/ })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '全选已制表' }));
    expect(screen.getByRole('button', { name: '导出所选题目（2）' })).toBeEnabled();
    fireEvent.click(screen.getByRole('button', { name: '清空选择' }));
    fireEvent.click(screen.getByRole('checkbox', { name: /导出 第二题/ }));
    fireEvent.click(screen.getByRole('button', { name: '导出所选题目（1）' }));
    await waitFor(() => expect(api.exportCases).toHaveBeenCalledWith(expect.objectContaining({ projectId: 'project-1', taskIds: ['task-2'], taskId: undefined, reviewedOnly: true })));
    await screen.findByText(/所选题目导出完成/);
    rerender(<AnnotationWorkspace projectId="project-2" taskId="task-1" />);
    await waitFor(() => expect(screen.getByRole('button', { name: '选择题目导出' })).toBeEnabled());
    fireEvent.click(screen.getByRole('button', { name: '选择题目导出' }));
    expect(screen.getByRole('button', { name: '导出所选题目（0）' })).toBeDisabled();
  });

  it('scopes the detail capture panel to the requested task without batch controls', async () => {
    api.listCases.mockResolvedValue([makeCase(), makeCase({ taskId: 'task-2', taskName: '当前详情题目', repoRelativePath: 'repo-two' })]);
    render(<AnnotationWorkspace projectId="project-1" taskId="task-2" />);
    expect(await screen.findByText('当前详情题目')).toBeInTheDocument();
    expect(screen.queryByText('低分也保留的任务')).not.toBeInTheDocument();
    expect(screen.queryByText('题目进度')).not.toBeInTheDocument();
    expect(screen.queryByText('批次预检与统一导出')).not.toBeInTheDocument();
    expect(await screen.findByDisplayValue('repo-two')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: '采集轨迹' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: '采集并准备制表数据' })).toBeInTheDocument();
    expect(api.listTraces).toHaveBeenCalledWith('task-2');
    api.captureAndPrepareTable.mockResolvedValue({ id: 'detail-capture', status: 'pending' });
    api.getAnnotationJob.mockResolvedValue({ id: 'detail-capture', status: 'done', outputPayload: JSON.stringify(makeCase({ taskId: 'task-2', taskName: '当前详情题目' })) });
    fireEvent.change(screen.getByLabelText('本机 JSONL 绝对路径'), { target: { value: '/tmp/task-two.jsonl' } });
    fireEvent.click(screen.getByRole('button', { name: '采集并准备制表数据' }));
    await waitFor(() => expect(api.captureAndPrepareTable).toHaveBeenCalledWith({ taskId: 'task-2', tracePath: '/tmp/task-two.jsonl' }));
    expect(await screen.findByText('采集并准备制表数据已完成')).toBeInTheDocument();
  });

  it('refreshes trace candidates even when the saved case revision has not changed', async () => {
    render(<AnnotationWorkspace projectId="project-1" taskId="task-1" />);
    await waitFor(() => expect(api.listTraces).toHaveBeenCalledTimes(1));
    api.listTraces.mockResolvedValue([{ path: '/new.jsonl', sessionId: 'new-session', size: 10 }]);
    fireEvent.click(screen.getByRole('button', { name: '刷新' }));
    expect(await screen.findByRole('option', { name: /new-session/ })).toBeInTheDocument();
  });

  it('does not fall back to another task when the detail task is missing', async () => {
    render(<AnnotationWorkspace projectId="project-1" taskId="missing-task" />);
    expect(await screen.findByText('当前题目暂无标注记录，请先确认题目已导入当前项目')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: '采集轨迹' })).not.toBeInTheDocument();
    expect(api.listTraces).not.toHaveBeenCalled();
  });

  it('exports saved evaluations for the whole project or a single card', async () => {
    api.exportCases.mockResolvedValue({ id: 'export-job', status: 'pending' });
    api.getAnnotationJob.mockResolvedValue({ id: 'export-job', status: 'done', outputPayload: JSON.stringify({ outputPath: '/exports/submission.xlsx', reportPath: '/exports/report.md', rows: 1, issues: [] }) });
    render(<AnnotationWorkspace projectId="project-1" />);
    await screen.findByText('题目进度');
    await waitFor(() => expect(screen.getByRole('button', { name: '一键导出已制表' })).toBeEnabled());
    fireEvent.click(screen.getByRole('button', { name: '一键导出已制表' }));
    await waitFor(() => expect(api.exportCases).toHaveBeenCalledWith(expect.objectContaining({ projectId: 'project-1', reviewedOnly: true, taskId: undefined })));
    await screen.findByText(/一键导出已制表完成/);
    fireEvent.click(screen.getByRole('button', { name: '导出本题 Excel' }));
    await waitFor(() => expect(api.exportCases).toHaveBeenCalledWith(expect.objectContaining({ projectId: 'project-1', reviewedOnly: true, taskId: 'task-1' })));
    await screen.findByText(/单题导出完成/);
  });

  it('prepares all project cases from one batch action and reports the summary', async () => {
    api.batchCaptureAndPrepareTable.mockResolvedValue({ id: 'batch-table-job', status: 'pending' });
    api.getAnnotationJob.mockResolvedValue({
      id: 'batch-table-job',
      status: 'done',
      outputPayload: JSON.stringify({ total: 3, prepared: 2, skipped: 1, failed: 0, items: [] }),
    });
    render(<AnnotationWorkspace projectId="project-1" />);
    const button = await screen.findByRole('button', { name: '批量采集并准备制表数据' });
    fireEvent.click(button);
    await waitFor(() => expect(api.batchCaptureAndPrepareTable).toHaveBeenCalledWith('project-1'));
    expect(await screen.findByText(/批量制表完成：新准备 2 题，跳过 1 题，失败 0 题/)).toBeInTheDocument();
    await waitFor(() => expect(api.listCases).toHaveBeenCalledTimes(2));
  });

  it('copies the startup command for the fixed task number in the detail panel', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    const clipboard = Object.getOwnPropertyDescriptor(navigator, 'clipboard');
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    try {
      api.listCases.mockResolvedValue([makeCase({ taskId: 'p1__feat__label-123-9', taskName: 'cyc-03', sourcePath: '/tasks/cyc-03-feature迭代-9' })]);
      render(<AnnotationWorkspace projectId="project-1" taskId="p1__feat__label-123-9" />);
      const copy = await screen.findByRole('button', { name: '复制容器启动命令' });
      await waitFor(() => expect(copy).toBeEnabled());
      fireEvent.click(copy);
      await waitFor(() => expect(writeText).toHaveBeenCalledWith(expect.stringContaining('CONTAINER_NAME="cyc03-claude-9"')));
      expect(writeText).toHaveBeenCalledWith(expect.stringContaining('RUN_DIR="$BASE_DIR/run-9"'));
      expect(await screen.findByText('容器启动命令已复制，请在本地终端执行')).toBeInTheDocument();
    } finally {
      if (clipboard) Object.defineProperty(navigator, 'clipboard', clipboard);
      else Reflect.deleteProperty(navigator, 'clipboard');
    }
  });

  it('shows approval and hides a stale repair prompt when all five scores are perfect', async () => {
    const item = makeCase();
    item.rounds[0].evaluations![0].scores = [5, 5, 5, 5, 5];
    api.listCases.mockResolvedValue([item]);
    render(<AnnotationWorkspace projectId="project-1" taskId="task-1" view="review" />);
    expect(await screen.findByText('五维满分 · 数据已保存')).toBeInTheDocument();
    expect(screen.queryByText('下一轮修复提示词（仅复制）')).not.toBeInTheDocument();
  });

  it('shows five-dimensional review in the detail review view and keeps batch export separate', async () => {
    render(<AnnotationWorkspace projectId="project-1" taskId="task-1" view="review" />);
    expect(await screen.findByText('真实轮次与评价')).toBeInTheDocument();
    expect(screen.getByText('1 分')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '重新审核' })).toBeInTheDocument();
    expect(screen.queryByText('批次预检与统一导出')).not.toBeInTheDocument();
  });

  it('shows every case and all five score cells without filtering low or missing scores', async () => {
    render(<AnnotationWorkspace projectId="project-1" projectName="标注项目" />);

    expect((await screen.findAllByText('低分也保留的任务')).length).toBeGreaterThan(0);
    expect(screen.getByText('1 分')).toBeInTheDocument();
    expect(screen.getByText('2 分')).toBeInTheDocument();
    expect(screen.getByText('待补证据')).toBeInTheDocument();
    expect(screen.getByText('独立验证输出')).toBeInTheDocument();
    expect(screen.getByText('边界未覆盖')).toBeInTheDocument();
    expect(screen.getByText('补充边界测试并记录结果')).toBeInTheDocument();
    expect(screen.queryByText(/配额|quota/i)).not.toBeInTheDocument();
  });

  it('renders legacy rounds whose evaluations field is null', async () => {
    const legacy = makeCase({
      rounds: [{
        ...makeCase().rounds[0],
        evaluations: null,
      }],
    });
    api.listCases.mockResolvedValue([legacy]);

    render(<AnnotationWorkspace projectId="project-1" />);

    expect((await screen.findAllByText('低分也保留的任务')).length).toBeGreaterThan(0);
    expect(screen.getByText('审核 0/1')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '审核本轮' })).toBeEnabled();
  });

  it('runs prepare through the job queue and reloads the returned case', async () => {
    const unprepared = makeCase({ initialSha: '', rounds: [], containerId: '', containerName: '' });
    const prepared = { ...unprepared, initialSha: 'prepared-sha', revision: 2 };
    api.listCases.mockResolvedValueOnce([unprepared]).mockResolvedValueOnce([prepared]);
    api.prepareCase.mockResolvedValue({ id: 'job-prepare', status: 'pending' });
    api.getAnnotationJob.mockResolvedValue({
      id: 'job-prepare',
      status: 'done',
      outputPayload: JSON.stringify(prepared),
    });

    render(<AnnotationWorkspace projectId="project-1" />);
    fireEvent.click(await screen.findByRole('button', { name: '准备题目' }));

    await waitFor(() => expect(api.prepareCase).toHaveBeenCalledWith('task-1'));
    await waitFor(() => expect(screen.getByText('prepared-sha')).toBeInTheDocument());
    expect(screen.getByText('准备题目已完成')).toBeInTheDocument();
    expect(api.getAnnotationJob).toHaveBeenCalledWith('job-prepare');
  });

  it('ignores a stale case response after the selected project changes', async () => {
    let resolveOld: ((value: AnnotationCase[]) => void) | undefined;
    api.listCases.mockImplementation((projectId: string) => {
      if (projectId === 'project-old') {
        return new Promise<AnnotationCase[]>((resolve) => { resolveOld = resolve; });
      }
      return Promise.resolve([makeCase({ taskId: 'task-new', projectId, taskName: '新项目任务' })]);
    });

    const view = render(<AnnotationWorkspace projectId="project-old" />);
    view.rerender(<AnnotationWorkspace projectId="project-new" />);

    expect((await screen.findAllByText('新项目任务')).length).toBeGreaterThan(0);
    resolveOld?.([makeCase({ taskId: 'task-old', projectId: 'project-old', taskName: '旧项目任务' })]);
    await Promise.resolve();

    expect(screen.queryByText('旧项目任务')).not.toBeInTheDocument();
    expect(screen.getAllByText('新项目任务').length).toBeGreaterThan(0);
  });

  it('requires a clean preflight for formal export and forwards the entered submission time unchanged', async () => {
    api.listCases.mockResolvedValue([makeCase({ completed: true })]);
    api.preflight.mockResolvedValue({ tasks: 1, rounds: 1, ready: 1, issues: [] });
    api.exportCases.mockResolvedValue({ id: 'job-export', status: 'pending' });
    api.getAnnotationJob.mockResolvedValue({
      id: 'job-export',
      status: 'done',
      outputPayload: JSON.stringify({ outputPath: '/out/batch.xlsx', reportPath: '/out/report.json', rows: 1, issues: [] }),
    });

    render(<AnnotationWorkspace projectId="project-1" />);
    expect(await screen.findByText('正式导出前请先执行批次预检')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: '正式导出' })).not.toBeInTheDocument();

    fireEvent.change(screen.getByLabelText('提交人'), { target: { value: '真实提交人' } });
    fireEvent.change(screen.getByLabelText('实际提交时间（可选）'), { target: { value: '2026-09-12T09:30' } });
    fireEvent.click(screen.getByRole('button', { name: '批次预检' }));
    fireEvent.click(await screen.findByRole('button', { name: '正式导出' }));

    await waitFor(() => expect(api.exportCases).toHaveBeenCalledWith({
      projectId: 'project-1',
      submitter: '真实提交人',
      submittedAt: '2026-09-12T09:30',
      draft: false,
    }));
    expect(await screen.findByText('已导出 1 行')).toBeInTheDocument();
  });

  it('accepts a local absolute JSONL path when container discovery has no matching trace', async () => {
    api.listTraces.mockResolvedValue([
      { path: '/home/node/.claude/projects/a.jsonl', sessionId: 'a', size: 10 },
      { path: '/home/node/.claude/projects/b.jsonl', sessionId: 'b', size: 20 },
    ]);
    api.captureAndPrepareTable.mockResolvedValue({ id: 'job-capture', status: 'pending' });
    api.getAnnotationJob.mockResolvedValue({
      id: 'job-capture',
      status: 'done',
      outputPayload: JSON.stringify(makeCase({ tracePath: '/tmp/manual.jsonl' })),
    });

    render(<AnnotationWorkspace projectId="project-1" />);
    const localPath = await screen.findByLabelText('本机 JSONL 绝对路径');
    fireEvent.change(localPath, { target: { value: '/tmp/manual.jsonl' } });
    fireEvent.click(screen.getByRole('button', { name: '采集并准备制表数据' }));

    await waitFor(() => expect(api.captureAndPrepareTable).toHaveBeenCalledWith({
      taskId: 'task-1',
      tracePath: '/tmp/manual.jsonl',
    }));
  });

  it('keeps a running action scoped to its case', async () => {
    api.listCases.mockResolvedValue([
      makeCase({ taskId: 'task-1', taskName: '第一题', initialSha: '', containerId: '', rounds: [] }),
      makeCase({ taskId: 'task-2', taskName: '第二题', initialSha: '', containerId: '', rounds: [] }),
    ]);
    api.prepareCase.mockResolvedValue({ id: 'job-one', status: 'pending' });
    api.getAnnotationJob.mockReturnValue(new Promise(() => {}));

    render(<AnnotationWorkspace projectId="project-1" />);
    fireEvent.click(await screen.findByRole('button', { name: '准备题目' }));
    await waitFor(() => expect(api.prepareCase).toHaveBeenCalledWith('task-1'));
    fireEvent.click(screen.getByRole('button', { name: /第二题/ }));

    expect(screen.getByRole('button', { name: '准备题目' })).toBeEnabled();
  });

  it('does not apply an old settings response after switching projects', async () => {
    let resolveSave: ((value: AnnotationCase) => void) | undefined;
    api.listCases.mockImplementation((projectId: string) => Promise.resolve([
      makeCase({
        taskId: projectId === 'project-old' ? 'task-old' : 'task-new',
        projectId,
        taskName: projectId === 'project-old' ? '旧设置任务' : '新设置任务',
        snapshotUrl: projectId === 'project-old' ? '' : 'https://example.test/new',
      }),
    ]));
    api.saveCaseSettings.mockReturnValue(new Promise<AnnotationCase>((resolve) => { resolveSave = resolve; }));

    const view = render(<AnnotationWorkspace projectId="project-old" />);
    await screen.findAllByText('旧设置任务');
    fireEvent.change(screen.getByLabelText('初始快照 URL'), { target: { value: 'https://example.test/old' } });
    fireEvent.click(screen.getByRole('button', { name: '保存题目设置' }));
    view.rerender(<AnnotationWorkspace projectId="project-new" />);

    expect((await screen.findAllByText('新设置任务')).length).toBeGreaterThan(0);
    resolveSave?.(makeCase({ taskId: 'task-old', projectId: 'project-old', snapshotUrl: 'https://example.test/old' }));
    await waitFor(() => expect(screen.getByLabelText('初始快照 URL')).toHaveValue('https://example.test/new'));
    expect(screen.queryByText('题目设置已保存')).not.toBeInTheDocument();
  });
});
