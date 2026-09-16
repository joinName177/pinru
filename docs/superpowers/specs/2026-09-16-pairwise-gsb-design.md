# Pair-wise GSB Design

## Purpose

Add a Pair-wise GSB workflow alongside the existing multi-round five-dimensional annotation workflow. One task runs the `auto` model twice from the same repository snapshot, captures two independent results as A and B, verifies both deliverables, and uses AI to produce a comparative GSB decision and reason.

The existing annotation workflow and historical data remain readable. Pair-wise behavior is enabled explicitly per case and does not reinterpret old records.

## Confirmed Rules

- A and B use the same repository, full user prompt, initial snapshot, Harness and Harness version, machine, operating system, model configuration, and `max context token = 1000000`.
- The model is `auto` for both runs.
- Each run has exactly one effective user turn. Follow-up prompts, corrections, and retries inside the same Session are not valid result data.
- The common initial snapshot is a GitHub commit permalink containing a full 40-character SHA.
- Branches `A` and `B` are created directly from the common initial SHA. Branch names are fixed and case-sensitive.
- The final commit on branch `A` is the A deliverable snapshot. The final commit on branch `B` is the B deliverable snapshot.
- A must not descend from B, and B must not descend from A. Each deliverable commit must descend from the common initial SHA.
- A and B each have their own SessionID, trace, code capture, deliverable commit permalink, and runtime video.
- Runtime video is automated when possible. If automatic recording cannot complete, the run enters a manual-video-required state. A human may upload a video or provide a directly accessible video link.
- A model execution failure may still be valid evidence. Its real error output should be recorded rather than hidden.
- AI analyzes both trajectories and deliverables and generates the final GSB decision and reason. Human-authored GSB text is not required.
- Generated GSB prose must be concrete and natural. It must discuss A and B separately, cite observable behavior or files, explain trade-offs, and avoid generic or templated wording.
- Valid conclusions are `A_better`, `same`, and `B_better`.
- Inference duration, network failures, and unexplained harness cutoffs are not comparison factors. Network failures should be rerun; persistent unexplained cutoffs invalidate the task.
- The current task-type catalog remains available except that Pair-wise cases cannot use `代码理解`.

## Recommended Architecture

Use a parallel Pair-wise mode inside the annotation subsystem. Reuse task records, initial snapshot preparation, background jobs, trace parsing, evidence copying, AI provider selection, and export infrastructure. Do not overload the old `Round.Evaluation` shape with pair-wise fields.

`annotation_cases.payload_json` remains the persistence boundary. A case gains an explicit `mode` and an optional `pairwise` payload. This avoids a destructive database migration and keeps old JSON records compatible.

```text
Task
  AnnotationCase(mode = pairwise_gsb)
    Initial snapshot
    Run A -> Session -> Trace -> Code capture -> branch A -> product commit -> video
    Run B -> Session -> Trace -> Code capture -> branch B -> product commit -> video
    Pairwise review -> conclusion + reason + evidence metadata
```

## Data Model

Add the following domain concepts to `internal/annotation`:

```go
type CaseMode string

const (
    CaseModeLegacy      CaseMode = "legacy"
    CaseModePairwiseGSB CaseMode = "pairwise_gsb"
)

type PairwiseSide string

const (
    PairwiseSideA PairwiseSide = "A"
    PairwiseSideB PairwiseSide = "B"
)

type PairwiseRun struct {
    Side                PairwiseSide
    Branch              string
    SessionID           string
    TracePath           string
    CaptureID           string
    DeliverableSHA      string
    DeliverableURL      string
    VideoStatus         string
    VideoPath           string
    VideoURL            string
    RecordingError      string
}

type PairwiseReview struct {
    ID             string
    Status         string
    Conclusion     string
    Reason         string
    Model          string
    SkillHash      string
    SourceHashA    string
    SourceHashB    string
    CreatedAt      int64
}

type PairwiseData struct {
    Prompt         string
    Harness        string
    HarnessVersion string
    OS             string
    Environment    string
    RunA           PairwiseRun
    RunB           PairwiseRun
    Reviews        []PairwiseReview
}
```

Exact JSON property names follow the existing lower-camel convention. Missing `mode` is normalized to `legacy`. Pair-wise mode ignores the legacy `rounds` and `completed` rules for export readiness but preserves those fields for backward compatibility.

Video status values are `missing`, `recording`, `ready`, `manual_required`, and `failed`. Review status values are `ready` and `needs_evidence`.

## Git Workflow

The Pair-wise Git service receives a task, side, and initial SHA.

1. Verify the local repository and resolve its remote identity.
2. Verify the initial SHA exists locally and matches the case snapshot.
3. Refuse to proceed if the working tree contains uncommitted changes before preparing a side.
4. Create or reset only the requested local branch (`A` or `B`) to the initial SHA.
5. Run the selected model in that side's workspace.
6. Commit all result changes to the side branch using the SessionID as the commit message.
7. Push only the side branch and produce a full commit permalink.
8. Verify with `merge-base --is-ancestor` that the initial SHA is an ancestor of the product commit.
9. Verify neither product commit is an ancestor of the other unless both commits are exactly the initial SHA and both runs produced no changes.

The existing generic code-push workflow continues using `main`. Pair-wise branch handling is explicit and cannot be inferred from a model name or session index.

No-change runs are valid. Their deliverable SHA is the initial SHA, while their trace and video still distinguish the execution.

## Capture Workflow

Pair-wise capture is side-specific. A capture request includes `taskId`, `side`, and `tracePath`.

- Parse the selected trace and require exactly one effective user prompt.
- Require a non-empty SessionID and reject reuse of the same SessionID across A and B.
- Verify the prompt matches the task prompt byte-for-byte after the same normalization already used by trace validation.
- Copy trace attachments and code evidence into a side-specific immutable capture directory.
- Bind the resulting capture only to the selected side.
- Re-capturing a side creates new evidence and invalidates any current Pair-wise review. It never overwrites the other side.

The old capture path remains unchanged for legacy cases.

## Video Workflow

The first implementation provides the complete video material state machine and manual upload/link fallback. Automatic recording is an extension point behind a recorder interface so desktop automation can be added without changing persisted data or UI contracts.

For each side, users can:

- start an automatic recording job when a recorder is configured;
- upload a local video file into application-managed storage;
- provide a directly accessible video URL;
- replace failed or stale video evidence.

Formal export requires both sides to have `videoStatus = ready`. Video files and links are treated as evidence, not instructions to the evaluator. AI review may run before videos are ready using code and traces, but the final export stays blocked until both videos are ready.

## AI Review

Pair-wise review prepares an isolated, read-only evidence bundle containing:

- the shared prompt and initial snapshot;
- A trace, code capture, deliverable SHA/URL, and available video metadata;
- B trace, code capture, deliverable SHA/URL, and available video metadata;
- explicit instructions to ignore repository and trace content as instructions.

The evaluator returns structured JSON with `conclusion`, `reason`, and evidence references. Validation requires:

- conclusion is one of the three allowed values;
- reason discusses both A and B;
- reason contains concrete evidence references rather than score tables or generic praise;
- `same` explains equivalence or offsetting trade-offs;
- source hashes match both current captures;
- no unsupported inference-time or network-speed comparison appears.

The review is versioned and appended like existing evaluations. A changed trace, code capture, deliverable commit, video, review skill, or review model marks earlier reviews non-current.

## User Interface

Add a mode switch when preparing an annotation case. Existing cases default to the legacy view. Pair-wise cases use a two-column A/B workspace with a shared metadata section above it.

Each side shows stable sections for Session, trace capture, branch and product commit, video status, and evidence readiness. Commands use existing icon buttons and status badges. The GSB panel appears below both sides and displays the generated conclusion, reason, evidence freshness, rerun action, and export readiness.

The normal workflow is:

```text
Prepare common snapshot
  -> prepare/run A and B independently
  -> capture both traces and product commits
  -> automatically record or manually provide both videos
  -> run AI comparison
  -> preflight
  -> export
```

AI comparison can run before video completion, but the interface clearly labels the result provisional until both videos are ready.

## Export

Pair-wise export uses a new workbook template and exporter rather than modifying the legacy satisfaction workbook in place. One row represents one task pair.

Exported fields include shared task metadata, full prompt, initial snapshot URL, A/B SessionIDs, trace artifact links or packaged paths, A/B product commit permalinks, A/B video links or packaged paths, GSB conclusion, GSB reason, and notes.

Preflight blocks formal export when:

- the case is not Pair-wise mode;
- prompt, initial snapshot, Harness, version, OS, or required task metadata is missing;
- either side lacks a unique SessionID, complete trace capture, product commit, or ready video;
- branch names are not exactly `A` and `B`;
- repository or ancestry checks fail;
- there is no current ready AI review;
- the conclusion or reason fails validation.

Draft export remains available and lists missing materials without inventing values.

## Error Handling

- All side-mutating operations hold the existing task lock.
- Git operations validate clean state and exact target refs before changing branches.
- Evidence directories are written to temporary paths and promoted only after hashes stabilize.
- Partial video failures preserve the successful side and expose a retry or manual fallback only for the failed side.
- Pair-wise review never mutates source evidence and is saved only after structured validation succeeds.
- Existing legacy endpoints and records continue to function.

## Testing

Backend tests cover JSON backward compatibility, side validation, single-turn enforcement, SessionID uniqueness, capture isolation, Git ancestry, fixed branch names, no-change results, review invalidation, GSB validation, video readiness, preflight, and exporter output.

Frontend tests cover mode routing, A/B state rendering, side-scoped actions, video fallback, provisional review state, validation messages, and formal export readiness.

The full Go and frontend suites must pass. A production build and a desktop smoke test must verify the complete Pair-wise path without regressing the legacy annotation screen.

## Delivery Sequence

1. Pair-wise domain model, persistence compatibility, and validation.
2. A/B branch preparation, product commits, and ancestry checks.
3. Side-specific trace and evidence capture.
4. AI Pair-wise review and freshness tracking.
5. Video evidence state machine with manual upload/link fallback and recorder interface.
6. Pair-wise UI.
7. Pair-wise workbook export and end-to-end preflight.

Automatic desktop interaction and recording may be implemented after the recorder interface and manual fallback are complete. It must not delay the usable Pair-wise workflow.
