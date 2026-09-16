# Pair-wise GSB MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver a usable Pair-wise GSB mode that stores independent A/B evidence, commits fixed A/B branches from one initial SHA, accepts manual video links, generates an AI comparison, validates readiness, and exports a pair-wise workbook without regressing legacy annotations.

**Architecture:** Extend `annotation_cases.payload_json` with an explicit mode and a separate pair-wise payload. Add side-specific service methods for Git preparation/commit, trace capture, material settings, and AI review, then expose them through existing background jobs and a focused A/B workspace in the annotation UI. Keep legacy round evaluation and export code unchanged; pair-wise validation and export live in separate files.

**Tech Stack:** Go, SQLite JSON persistence, Git CLI, Wails v3 services/jobs, React 19, TypeScript, Vitest, Python/openpyxl exporter.

**Spec:** `docs/superpowers/specs/2026-09-16-pairwise-gsb-design.md`

## Global Constraints

- A and B use one repository, one prompt, and one full 40-character initial SHA.
- Branch names are exactly `A` and `B` and both start at the initial SHA.
- Each side has one unique SessionID and exactly one effective user turn.
- Each side stores its own trace capture, deliverable commit, and video material.
- AI generates `A_better`, `same`, or `B_better` plus a concrete reason discussing both sides.
- Formal readiness requires both videos; the MVP accepts directly accessible video URLs and leaves automatic recording behind a future recorder interface.
- Legacy annotation behavior and historical JSON remain compatible.

---

### Task 1: Pair-wise Domain Model and Validation

**Files:**
- Modify: `internal/annotation/types.go`
- Create: `internal/annotation/pairwise.go`
- Create: `internal/annotation/pairwise_test.go`
- Modify: `internal/store/annotation.go`

**Interfaces:**
- Produces: `CaseMode`, `PairwiseSide`, `PairwiseRun`, `PairwiseReview`, `PairwiseData`, `NormalizeCase(*Case)`, `ValidatePairwiseCase(Case, bool) []string`, `CurrentPairwiseReview(Case) *PairwiseReview`.
- Persistence continues through `Store.SaveAnnotationCase` and `Store.GetAnnotationCase`.

- [ ] **Step 1: Write failing compatibility and validation tests**

```go
func TestPairwiseCaseRoundTripAndLegacyDefault(t *testing.T) { /* JSON and store round trip */ }
func TestValidatePairwiseCaseRequiresDistinctSingleTurnSides(t *testing.T) { /* sessions, captures, commits */ }
func TestValidatePairwiseFormalRequiresVideosAndCurrentReview(t *testing.T) { /* draft vs formal */ }
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/annotation ./internal/store -run 'Pairwise|LegacyDefault'`
Expected: compile failure because Pair-wise types and functions do not exist.

- [ ] **Step 3: Implement model, normalization, and pure validation**

Add `Mode CaseMode` and `Pairwise *PairwiseData` to `Case`. Normalize empty mode to `legacy`, initialize slices, and keep historical fields untouched. Validation checks fixed branches, unique sessions, one parsed round per side, 40-character SHAs, capture hashes, videos, and current review source hashes.

- [ ] **Step 4: Run focused and package tests**

Run: `go test ./internal/annotation ./internal/store`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/annotation internal/store/annotation.go
git commit -m "feat(annotation): add pairwise case model"
```

### Task 2: Fixed A/B Git Workflow

**Files:**
- Create: `internal/gitops/pairwise.go`
- Create: `internal/gitops/pairwise_test.go`
- Create: `app/annotation/pairwise_git.go`
- Create: `app/annotation/pairwise_git_test.go`

**Interfaces:**
- Produces: `gitops.PreparePairwiseBranch(ctx, path, side, initialSHA) error`, `gitops.CommitPairwiseResult(ctx, path, side, initialSHA, message) (sha string, error)`, `AnnotationService.PreparePairwiseSide`, and `AnnotationService.CommitPairwiseSide`.

- [ ] **Step 1: Write failing repository topology tests**

```go
func TestPreparePairwiseBranchesStartAtSameInitialSHA(t *testing.T) { /* A and B refs */ }
func TestCommitPairwiseResultRejectsWrongBranchAndDirtyPreparation(t *testing.T) { /* safety */ }
func TestCommitPairwiseNoChangesUsesInitialSHA(t *testing.T) { /* valid no-op */ }
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/gitops ./app/annotation -run Pairwise`
Expected: compile failure because Pair-wise Git functions do not exist.

- [ ] **Step 3: Implement side-safe Git operations**

Use `git rev-parse`, `status --porcelain`, `switch -C <side> <initialSHA>`, `add -A`, `commit`, and `merge-base --is-ancestor`. Do not alter the generic `main` code-push path. Save side branch, deliverable SHA, and commit URL into the Pair-wise case.

- [ ] **Step 4: Run focused tests**

Run: `go test ./internal/gitops ./app/annotation -run Pairwise`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/gitops/pairwise* app/annotation/pairwise_git*
git commit -m "feat(annotation): add fixed pairwise branches"
```

### Task 3: Side-specific Trace Capture and Video Materials

**Files:**
- Create: `app/annotation/pairwise_capture.go`
- Create: `app/annotation/pairwise_capture_test.go`
- Modify: `app/annotation/service.go`
- Modify: `app/job/service.go`

**Interfaces:**
- Produces: `PairwiseCaptureRequest`, `PairwiseMaterialsRequest`, `AnnotationService.CapturePairwiseSide`, and `AnnotationService.SavePairwiseMaterials`.
- Jobs: `annotation_pairwise_capture`, `annotation_pairwise_materials`, `annotation_pairwise_prepare_side`, `annotation_pairwise_commit_side`.

- [ ] **Step 1: Write failing side isolation tests**

```go
func TestCapturePairwiseSideStoresOneTurnWithoutReplacingOtherSide(t *testing.T) { /* immutable captures */ }
func TestCapturePairwiseSideRejectsDuplicateSessionAndMultipleTurns(t *testing.T) { /* validity */ }
func TestSavePairwiseMaterialsRequiresHTTPVideoURL(t *testing.T) { /* manual fallback */ }
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./app/annotation ./app/job -run Pairwise`
Expected: compile failure because Pair-wise requests and handlers do not exist.

- [ ] **Step 3: Implement capture, materials, and job routing**

Reuse `traceFiles`, `ParseTrace`, `validateTraceSource`, `CopyEvidenceTree`, and task locks. Require exactly one non-excluded parsed round. Copy code and trace into `captures/<id>` and bind only the selected side. Save a valid HTTP(S) video URL as `ready`; empty URL resets to `missing`. Any evidence change makes old Pair-wise reviews stale via source hashes.

- [ ] **Step 4: Run focused tests**

Run: `go test ./app/annotation ./app/job -run Pairwise`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/annotation/pairwise_capture* app/annotation/service.go app/job/service.go
git commit -m "feat(annotation): capture pairwise evidence"
```

### Task 4: AI Pair-wise GSB Review

**Files:**
- Create: `app/cli/pairwise_review.go`
- Create: `app/cli/pairwise_review_test.go`
- Create: `app/annotation/pairwise_review.go`
- Create: `app/annotation/pairwise_review_test.go`
- Modify: `app/annotation/service.go`

**Interfaces:**
- Produces: `PairwiseReviewRequest`, `CliService.RunPairwiseReview`, `AnnotationService.ReviewPairwise`, and job `annotation_pairwise_review`.

- [ ] **Step 1: Write failing parser and review freshness tests**

```go
func TestParsePairwiseReviewAcceptsConcreteComparison(t *testing.T) { /* structured JSON */ }
func TestParsePairwiseReviewRejectsGenericOrOneSidedReason(t *testing.T) { /* quality */ }
func TestReviewPairwiseInvalidatesWhenEitherEvidenceChanges(t *testing.T) { /* hashes */ }
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./app/cli ./app/annotation -run PairwiseReview`
Expected: compile failure because review interfaces do not exist.

- [ ] **Step 3: Implement isolated evidence bundle and structured evaluator**

Mirror the existing CLI/DeepSeek execution selection, but use a dedicated prompt and output schema. The result contains only conclusion, reason, status, and evidence references. Require explicit A and B discussion and reject five-dimensional score language and generic boilerplate.

- [ ] **Step 4: Run focused tests**

Run: `go test ./app/cli ./app/annotation -run PairwiseReview`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/cli/pairwise_review* app/annotation/pairwise_review* app/annotation/service.go
git commit -m "feat(annotation): generate pairwise gsb reviews"
```

### Task 5: Pair-wise API and User Interface

**Files:**
- Modify: `frontend/src/api/annotation.ts`
- Create: `frontend/src/features/annotation/PairwiseWorkspace.tsx`
- Create: `frontend/src/features/annotation/PairwiseWorkspace.test.tsx`
- Modify: `frontend/src/features/annotation/index.tsx`

**Interfaces:**
- Consumes Pair-wise fields and jobs from Tasks 1-4.
- Produces mode selection and a two-column A/B workflow with branch preparation, trace capture, commit, video URL, review, and readiness display.

- [ ] **Step 1: Write failing component tests**

```tsx
it('renders independent A and B evidence states', () => { /* two columns */ });
it('submits side-specific capture and video actions', async () => { /* API payload */ });
it('shows provisional review until both videos are ready', () => { /* readiness */ });
```

- [ ] **Step 2: Run tests and verify RED**

Run: `npm test -- --run src/features/annotation/PairwiseWorkspace.test.tsx`
Expected: FAIL because the component does not exist.

- [ ] **Step 3: Implement the Pair-wise workspace**

Use compact un-nested sections, existing buttons/icons, fixed control dimensions, and responsive two-column layout. Legacy cases continue rendering the existing workspace. Add a deliberate mode enable action that cannot silently convert a case containing legacy rounds.

- [ ] **Step 4: Run frontend tests and typecheck**

Run: `npm test -- --run src/features/annotation/PairwiseWorkspace.test.tsx`
Run: `npm run typecheck`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/api/annotation.ts frontend/src/features/annotation
git commit -m "feat(annotation): add pairwise gsb workspace"
```

### Task 6: Pair-wise Preflight and Workbook Export

**Files:**
- Create: `app/annotation/pairwise_export.go`
- Create: `app/annotation/pairwise_export_test.go`
- Create: `app/annotation/assets/export_pairwise.py`
- Create: `app/annotation/assets/tests/test_export_pairwise.py`
- Modify: `app/annotation/assets.go`
- Modify: `app/annotation/service.go`
- Modify: `frontend/src/api/annotation.ts`
- Modify: `frontend/src/features/annotation/PairwiseWorkspace.tsx`

**Interfaces:**
- Produces: `PreflightPairwise`, `ExportPairwise`, job `annotation_pairwise_export`, and an XLSX with one row per pair.

- [ ] **Step 1: Write failing preflight and workbook tests**

```go
func TestPairwisePreflightReportsMissingSideMaterials(t *testing.T) { /* actionable issues */ }
func TestPairwiseExportWritesOnePairPerRow(t *testing.T) { /* workbook contract */ }
```

```python
def test_export_pairwise_writes_shared_a_b_and_gsb_columns(tmp_path):
    output = run_export(tmp_path, complete_pairwise_input())
    workbook = openpyxl.load_workbook(output)
    sheet = workbook.active
    assert [cell.value for cell in sheet[1]][:6] == [
        "User Prompt", "初始环境快照", "A-SessionID",
        "A-产物快照", "B-SessionID", "B-产物快照",
    ]
    assert sheet.cell(2, 3).value == "session-a"
    assert sheet.cell(2, 5).value == "session-b"
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./app/annotation -run PairwiseExport`
Run: `python3 -m unittest app.annotation.assets.tests.test_export_pairwise`
Expected: FAIL because Pair-wise export does not exist.

- [ ] **Step 3: Implement independent Pair-wise preflight and exporter**

Generate the workbook from a JSON batch input using openpyxl. Include shared metadata, A/B SessionIDs, trace/capture references, product commit URLs, video URLs, conclusion, reason, and notes. Draft export includes issue text; formal export rejects incomplete cases.

- [ ] **Step 4: Run focused export tests**

Run: `go test ./app/annotation -run PairwiseExport`
Run: `python3 -m unittest app.annotation.assets.tests.test_export_pairwise`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/annotation frontend/src/api/annotation.ts frontend/src/features/annotation/PairwiseWorkspace.tsx
git commit -m "feat(annotation): export pairwise gsb workbook"
```

### Task 7: Full Verification and Documentation

**Files:**
- Modify: `llmdoc/guide/claude-container-annotation.md`
- Modify: `llmdoc/index.md`

**Interfaces:**
- Documents the complete MVP workflow and known automatic-recording boundary.

- [ ] **Step 1: Update user and architecture documentation**

Document mode selection, common snapshot, A/B branch order, side captures, video URL fallback, AI review, and export.

- [ ] **Step 2: Run formatting and static checks**

Run: `gofmt -w <changed-go-files>`
Run: `git diff --check`
Run: `cd frontend && npm run typecheck`
Expected: PASS.

- [ ] **Step 3: Run complete test suites**

Run: `go test ./...`
Run: `cd frontend && npm test -- --run`
Run: `cd frontend && npm run build`
Expected: PASS.

- [ ] **Step 4: Review the branch diff and commit documentation**

```bash
git add llmdoc
git commit -m "docs: describe pairwise gsb workflow"
```
