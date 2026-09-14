import { cancelJob, getJob, submitJob, type BackgroundJob } from './job';
import { callService } from './wails';

export type AnnotationRoundStatus = 'complete' | 'pending' | 'conflict' | 'excluded';

export interface AnnotationIssue {
  description: string;
  evidence: string;
  kind: string;
}

export interface AnnotationEvaluation {
	current?: boolean;
  requirementChecks?: Array<{requirement:string; status:'completed'|'failed'|'unverified'; evidence:string}>;
  id: string;
  createdAt: number;
  skillHash: string;
  model: string;
  evidenceHash: string;
  status: string;
  sourceHash?: string;
  reviewPath?: string;
  reviewHash?: string;
  scores: Array<number | null>;
  descriptions: string[];
  taskType: string;
  difficulty: string;
  language: string;
  environment: string;
  harnessVersion: string;
  os: string;
  evidence: string[];
  missing: string[];
  limitations?: string[];
  issues: AnnotationIssue[];
  nextPrompt: string;
  nextPromptType: string;
}

export interface AnnotationRound {
  promptId: string;
  sessionId: string;
  prompt: string;
  order: number;
  status: AnnotationRoundStatus;
  reason: string;
  evidenceHash: string;
  sourceStart: number;
  sourceEnd: number;
  version: string;
  cwd: string;
  captureId: string;
  evaluations: AnnotationEvaluation[] | null;
}

export interface AnnotationCapture {
  id: string;
  dir: string;
  tracePath: string;
  codePath: string;
  hash: string;
  traceHash?: string;
  createdAt: number;
}

export interface AnnotationCase {
	preparation?: AnnotationPreparation;
  taskId: string;
  projectId: string;
  taskName: string;
  sourcePath: string;
  initialSha: string;
  snapshotUrl: string;
  containerId: string;
  containerName: string;
  workspacePath: string;
  repoRelativePath: string;
  sessionId: string;
  tracePath: string;
  completed: boolean;
  rounds: AnnotationRound[];
  captures: AnnotationCapture[];
  revision: number;
  updatedAt: number;
}

export interface AnnotationPreparation {
  jobId: string;
  status: string;
  progress: number;
  message: string;
  error: string;
  startedAt: number;
  finishedAt: number;
  lastActivityAt: number;
}

export interface AnnotationContainer {
  id: string;
  name: string;
  state: string;
  image: string;
  workspacePath: string;
}

export interface TraceCandidate {
  path: string;
  sessionId: string;
  size: number;
}

export interface AnnotationPreflightReport {
  tasks: number;
  rounds: number;
  ready: number;
  issues: string[];
}

export interface AnnotationExportResult {
  outputPath: string;
  reportPath: string;
  rows: number;
  issues: string[];
}

export interface AnnotationBatchPrepareItem {
  taskId: string;
  taskName: string;
  status: 'prepared' | 'skipped' | 'failed';
  message: string;
}

export interface AnnotationBatchPrepareResult {
  total: number;
  prepared: number;
  skipped: number;
  failed: number;
  items: AnnotationBatchPrepareItem[];
}

export interface BindContainerRequest {
  taskId: string;
  containerId: string;
  repoRelativePath: string;
  copyRepository: boolean;
}

export interface CaptureRequest {
  taskId: string;
  tracePath: string;
}

export interface ReviewRequest {
  taskId: string;
  promptId: string;
  force: boolean;
}

export interface SaveCaseSettingsRequest {
  taskId: string;
  snapshotUrl: string;
  completed: boolean;
}

export interface ExportAnnotationRequest {
  taskIds?: string[];
  taskId?: string;
  reviewedOnly?: boolean;
  projectId: string;
  submitter: string;
  submittedAt: string;
  draft: boolean;
}

const JOB_OPTIONS = { maxRetries: 1, timeoutSeconds: 1800 } as const;

function submitAnnotationJob(jobType: string, taskId: string, input: unknown) {
  return submitJob({
    jobType,
    taskId,
    inputPayload: JSON.stringify(input),
    ...JOB_OPTIONS,
  });
}

export function listCases(projectId: string): Promise<AnnotationCase[]> {
  return callService('AnnotationService', 'ListCases', projectId);
}

export function listContainers(): Promise<AnnotationContainer[]> {
  return callService('AnnotationService', 'ListContainers');
}

export function listTraces(taskId: string): Promise<TraceCandidate[]> {
  return callService('AnnotationService', 'ListTraces', taskId);
}

export function saveCaseSettings(request: SaveCaseSettingsRequest): Promise<AnnotationCase> {
  return callService('AnnotationService', 'SaveCaseSettings', request);
}

export function preflight(projectId: string): Promise<AnnotationPreflightReport> {
  return callService('AnnotationService', 'Preflight', projectId);
}

export function prepareCase(taskId: string): Promise<BackgroundJob> {
  return submitAnnotationJob('annotation_prepare', taskId, { taskId });
}

export function captureAndPrepareTable(request: CaptureRequest): Promise<BackgroundJob> {
  return submitAnnotationJob('annotation_capture_table', request.taskId, request);
}

export function batchCaptureAndPrepareTable(projectId: string): Promise<BackgroundJob> {
  return submitJob({
    jobType: 'annotation_batch_capture_table',
    taskId: '',
    inputPayload: JSON.stringify({ projectId }),
    maxRetries: 1,
    timeoutSeconds: 21600,
  });
}

export function resumeTable(taskId: string): Promise<BackgroundJob> {
  return submitAnnotationJob('annotation_resume', taskId, { taskId });
}

export function bindContainer(request: BindContainerRequest): Promise<BackgroundJob> {
  return submitAnnotationJob('annotation_bind', request.taskId, request);
}

export function captureCase(request: CaptureRequest): Promise<BackgroundJob> {
  return submitAnnotationJob('annotation_capture', request.taskId, request);
}

export function reviewRound(request: ReviewRequest): Promise<BackgroundJob> {
  return submitAnnotationJob('annotation_review', request.taskId, request);
}

export function exportCases(request: ExportAnnotationRequest): Promise<BackgroundJob> {
  return submitAnnotationJob('annotation_export', '', request);
}

export const getAnnotationJob = getJob;
export const cancelAnnotationJob = cancelJob;

export function publishSnapshot(taskId: string): Promise<BackgroundJob> {
  return submitAnnotationJob('annotation_publish', taskId, { taskId });
}
