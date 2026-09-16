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
    prompt: '修复筛选', harness: 'Codex', harnessVersion: '1.0', os: 'MacOS/Linux', environment: '', notes: '', autoRecordEnabled: false,
    runA: { side: 'A', branch: 'A', sessionId: 'session-a', tracePath: '/a.jsonl', turnCount: 1, captureId: 'ca', captureHash: 'ha', traceHash: 'ta', deliverableSha: 'b'.repeat(40), deliverableUrl: 'https://github.com/u/r/commit/a', videoStatus: 'ready', videoPath: '', videoUrl: 'https://example.com/a.mp4', recordingError: '', preparedAt: 1, capturedAt: 2, committedAt: 3 },
    runB: { side: 'B', branch: 'B', sessionId: '', tracePath: '', turnCount: 0, captureId: '', captureHash: '', traceHash: '', deliverableSha: '', deliverableUrl: '', videoStatus: 'missing', videoPath: '', videoUrl: '', recordingError: '', preparedAt: 0, capturedAt: 0, committedAt: 0 },
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
  expect(within(sideA).getByText('session-a')).toBeInTheDocument();
  expect(within(sideA).getByText('视频已就绪')).toBeInTheDocument();
  expect(within(sideB).getByText('尚未采集')).toBeInTheDocument();
});

it('submits side-specific capture and video actions', async () => {
  const runJob = vi.fn(async (_label: string, submit: () => Promise<unknown>) => { await submit(); return true; });
  render(<PairwiseWorkspace annotationCase={pairwiseCase} disabled={false} runJob={runJob} />);
  const sideB = screen.getByRole('region', { name: '运行 B' });
  fireEvent.change(within(sideB).getByLabelText('B 轨迹路径'), { target: { value: '/tmp/b.jsonl' } });
  fireEvent.click(within(sideB).getByRole('button', { name: '采集 B' }));
  await waitFor(() => expect(api.capturePairwiseSide).toHaveBeenCalledWith({ taskId: 'task-1', side: 'B', tracePath: '/tmp/b.jsonl' }));

  fireEvent.change(within(sideB).getByLabelText('B 视频链接'), { target: { value: 'https://example.com/b.mp4' } });
  fireEvent.click(within(sideB).getByRole('button', { name: '保存 B 视频' }));
  await waitFor(() => expect(api.savePairwiseMaterials).toHaveBeenCalledWith({ taskId: 'task-1', side: 'B', videoUrl: 'https://example.com/b.mp4', recordingError: '' }));
});

it('shows provisional review until both videos are ready', () => {
  render(<PairwiseWorkspace annotationCase={pairwiseCase} disabled={false} runJob={vi.fn()} />);
  expect(screen.getByText('当前结论将保持临时状态，正式导出前需补齐 A/B 视频。')).toBeInTheDocument();
});
