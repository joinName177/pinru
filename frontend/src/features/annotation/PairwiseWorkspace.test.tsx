import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, expect, it, vi } from 'vitest';
import type { AnnotationCase } from '../../api/annotation';
import { PairwiseWorkspace } from './PairwiseWorkspace';

const api = vi.hoisted(() => ({
  capturePairwiseSide: vi.fn(),
  commitPairwiseSide: vi.fn(),
  preparePairwiseSide: vi.fn(),
  reviewPairwise: vi.fn(),
  savePairwiseMaterials: vi.fn(),
  savePairwiseSettings: vi.fn(),
}));

vi.mock('../../api/annotation', async () => ({
  ...await vi.importActual<typeof import('../../api/annotation')>('../../api/annotation'),
  ...api,
}));

const pairwiseCase = {
  taskId: 'task-1', projectId: 'project-1', taskName: 'Pair test', taskType: 'Bug修复',
  sourcePath: '/repo', initialSha: 'a'.repeat(40), snapshotUrl: `https://github.com/u/r/commit/${'a'.repeat(40)}`,
  containerId: '', containerName: '', workspacePath: '', repoRelativePath: 'repo', sessionId: '', tracePath: '',
  completed: false, rounds: [], captures: [], revision: 1, updatedAt: 1, mode: 'pairwise_gsb',
  pairwise: {
    prompt: '修复筛选', language: 'TypeScript', harness: 'Codex CLI', harnessVersion: '1.0', os: 'MacOS/Linux', environment: '', notes: '', validity: '有效', autoRecordEnabled: false,
    runA: { side: 'A', branch: 'A', containerId: 'container-a', containerName: 'claude-a', workspacePath: '/workspace-a', repoRelativePath: 'repo', sessionId: 'session-a', tracePath: '/a.jsonl', turnCount: 1, captureId: 'ca', captureHash: 'ha', traceHash: 'ta', deliverableSha: 'b'.repeat(40), deliverableUrl: 'https://github.com/u/r/commit/a', videoStatus: 'ready', videoPath: '', videoUrl: 'https://example.com/a.mp4', recordingError: '', preparedAt: 1, capturedAt: 2, committedAt: 3 },
    runB: { side: 'B', branch: 'B', containerId: 'container-b', containerName: 'claude-b', workspacePath: '/workspace-b', repoRelativePath: 'repo', sessionId: '', tracePath: '', turnCount: 0, captureId: '', captureHash: '', traceHash: '', deliverableSha: '', deliverableUrl: '', videoStatus: 'missing', videoPath: '', videoUrl: '', recordingError: '', preparedAt: 0, capturedAt: 0, committedAt: 0 },
    reviews: [],
  },
} as AnnotationCase;

beforeEach(() => {
  vi.clearAllMocks();
  Object.values(api).forEach((fn) => fn.mockResolvedValue({ id: 'job-1', status: 'pending' }));
});

it('renders independent A and B evidence states', () => {
  render(<PairwiseWorkspace annotationCase={pairwiseCase} disabled={false} runJob={vi.fn()} />);
  const sideA = screen.getByRole('region', { name: '运行 A' });
  const sideB = screen.getByRole('region', { name: '运行 B' });
  expect(within(sideA).queryByRole('button', { name: '准备 A' })).not.toBeInTheDocument();
  expect(within(sideB).queryByRole('button', { name: '准备 B' })).not.toBeInTheDocument();
  expect(within(sideA).getByText('session-a')).toBeInTheDocument();
  expect(within(sideA).getByText('视频已就绪')).toBeInTheDocument();
  expect(within(sideA).getByRole('button', { name: '已提交 A' })).toBeEnabled();
  expect(within(sideB).getByText('尚未采集')).toBeInTheDocument();
  expect(within(sideB).getByRole('button', { name: '提交 B 产物' })).toBeDisabled();
});

it('keeps refresh and binding actions while showing automatic prompt-copy state', async () => {
  const onRefreshContainers = vi.fn().mockResolvedValue(undefined);
  const onBindContainer = vi.fn().mockResolvedValue(true);
  render(<PairwiseWorkspace
    annotationCase={pairwiseCase}
    containers={[{ id: 'container-a', name: 'pair-claude-1-a', state: 'running', image: 'claude', workspacePath: '/workspace-a' }]}
    disabled={false}
    runJob={vi.fn()}
    onRefreshContainers={onRefreshContainers}
    onBindContainer={onBindContainer}
    promptCopied={{ A: true, B: false }}
  />);
  const sideA = screen.getByRole('region', { name: '运行 A' });
  const buttons = within(sideA).getAllByRole('button').map((button) => button.textContent?.trim());
  expect(buttons.indexOf('刷新容器')).toBeLessThan(buttons.indexOf('已绑定 A'));
  expect(within(sideA).getByRole('button', { name: '已绑定 A' })).toBeEnabled();
  expect(within(sideA).getByText('提示词已复制')).toBeInTheDocument();
  expect(within(sideA).queryByRole('button', { name: '提示词已复制' })).not.toBeInTheDocument();
  expect(within(sideA).queryByRole('button', { name: '复制同一提示词' })).not.toBeInTheDocument();
  fireEvent.click(within(sideA).getByRole('button', { name: '刷新容器' }));
  fireEvent.click(within(sideA).getByRole('button', { name: '已绑定 A' }));
  await waitFor(() => expect(onRefreshContainers).toHaveBeenCalledWith('A'));
  expect(onBindContainer).toHaveBeenCalledWith('A', 'container-a');
});

it('submits side-specific capture and video actions', async () => {
  const runJob = vi.fn(async (_label: string, submit: () => Promise<unknown>) => { await submit(); return true; });
  render(<PairwiseWorkspace annotationCase={pairwiseCase} disabled={false} runJob={runJob} />);
  const sideB = screen.getByRole('region', { name: '运行 B' });
  expect(within(sideB).queryByLabelText('B 轨迹路径')).not.toBeInTheDocument();
  fireEvent.click(within(sideB).getByRole('button', { name: '采集 B' }));
  await waitFor(() => expect(api.capturePairwiseSide).toHaveBeenCalledWith({ taskId: 'task-1', side: 'B' }));

  fireEvent.change(within(sideB).getByLabelText('B 视频链接'), { target: { value: 'https://example.com/b.mp4' } });
  fireEvent.click(within(sideB).getByRole('button', { name: '保存 B 视频' }));
  await waitFor(() => expect(api.savePairwiseMaterials).toHaveBeenCalledWith({ taskId: 'task-1', side: 'B', videoUrl: 'https://example.com/b.mp4', videoPath: '', recordingError: '' }));
});

it('starts and stops the committed project for each side', async () => {
  const onStartProject = vi.fn().mockResolvedValue({ side: 'A', running: true, url: 'http://192.168.1.2:4173', command: 'pnpm dev' });
  const onStopProject = vi.fn().mockResolvedValue(true);
  render(<PairwiseWorkspace annotationCase={pairwiseCase} disabled={false} runJob={vi.fn()} onStartProject={onStartProject} onStopProject={onStopProject} />);
  const sideA = screen.getByRole('region', { name: '运行 A' });
  fireEvent.click(within(sideA).getByRole('button', { name: '启动项目 A' }));
  await waitFor(() => expect(onStartProject).toHaveBeenCalledWith('A'));
  expect(within(sideA).getByText('项目运行中')).toBeInTheDocument();
  expect(within(sideA).getByRole('link', { name: '打开 A 项目' })).toHaveAttribute('href', 'http://192.168.1.2:4173');
  fireEvent.click(within(sideA).getByRole('button', { name: '停止项目 A' }));
  await waitFor(() => expect(onStopProject).toHaveBeenCalledWith('A'));
  await waitFor(() => expect(within(sideA).queryByRole('link', { name: '打开 A 项目' })).not.toBeInTheDocument());
});

it('does not expose automatic recording buttons', () => {
  render(<PairwiseWorkspace annotationCase={pairwiseCase} disabled={false} runJob={vi.fn()} />);
  const sideA = screen.getByRole('region', { name: '运行 A' });
  const sideB = screen.getByRole('region', { name: '运行 B' });
  expect(within(sideA).queryByRole('button', { name: '录制 A 视频' })).not.toBeInTheDocument();
  expect(within(sideB).queryByRole('button', { name: '录制 B 视频' })).not.toBeInTheDocument();
});

it('shows a side-specific recording guide and can regenerate it', async () => {
  const onGenerateRecordingGuide = vi.fn().mockResolvedValue(true);
  const withGuide = structuredClone(pairwiseCase);
  withGuide.pairwise!.runA.recordingGuide = ['打开项目首页', '点击皱眉榜', '打开第一条记录查看详情'];
  render(<PairwiseWorkspace annotationCase={withGuide} disabled={false} runJob={vi.fn()} onGenerateRecordingGuide={onGenerateRecordingGuide} />);
  const sideA = screen.getByRole('region', { name: '运行 A' });
  expect(within(sideA).getByText('点击皱眉榜')).toBeInTheDocument();
  expect(within(sideA).getByText('轨迹已采集')).toBeInTheDocument();
  expect(within(sideA).getByText('产物已提交')).toBeInTheDocument();
  expect(within(sideA).getByText('指引已生成')).toBeInTheDocument();
  fireEvent.click(within(sideA).getByRole('button', { name: '重新生成 A 录制指引' }));
  await waitFor(() => expect(onGenerateRecordingGuide).toHaveBeenCalledWith('A'));
});

it('shows provisional review until both videos are ready', () => {
  render(<PairwiseWorkspace annotationCase={pairwiseCase} disabled={false} runJob={vi.fn()} />);
  expect(screen.getByText('当前结论将保持临时状态，正式导出前需补齐 A/B 视频。')).toBeInTheDocument();
});

it('does not show separately editable submission metadata', () => {
  render(<PairwiseWorkspace annotationCase={pairwiseCase} disabled={false} runJob={vi.fn()} />);
  expect(screen.queryByLabelText('语言/框架')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('环境可复现等级')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('有效性')).not.toBeInTheDocument();
  expect(screen.queryByText('Harness')).not.toBeInTheDocument();
});

it('offers draft and formal pairwise export actions', async () => {
  const onExport = vi.fn().mockResolvedValue(undefined);
  render(<PairwiseWorkspace annotationCase={pairwiseCase} disabled={false} runJob={vi.fn()} onExport={onExport} />);
  fireEvent.click(screen.getByRole('button', { name: '导出 Pair-wise 草稿' }));
  await waitFor(() => expect(onExport).toHaveBeenCalledWith(true));
  expect(screen.getByRole('button', { name: '正式导出 Pair-wise' })).toBeDisabled();
});
