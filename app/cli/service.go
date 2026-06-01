package cli

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/blueship581/pinru/internal/errs"
	"github.com/blueship581/pinru/internal/util"
	"github.com/google/uuid"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed manuals/*
var manualFS embed.FS

//go:embed schemas/pg_code_review.json
var pgCodeReviewSchema []byte

// Service executes the local claude CLI and streams output back via polling.
type CliService struct {
	mu                sync.Mutex
	sessions          map[string]*cliSession
	resolveCLI        func(name string) (string, error) // nil → util.ResolveCLI
	reviewContextPath string
}

func defaultPgCodeContextScriptPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, ".codex", "skills", "pg-code", "scripts", "collect_project_context.py"),
		filepath.Join(home, ".claude", "skills", "pg-code", "scripts", "collect_project_context.py"),
	}
}

type cliSession struct {
	mu       sync.Mutex
	lines    []string
	done     bool
	exitErr  string
	cancel   context.CancelFunc
	lastUsed time.Time
}

func (s *cliSession) append(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines = append(s.lines, line)
	s.lastUsed = time.Now()
}

func (s *cliSession) finish(errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.done = true
	s.exitErr = errMsg
	s.lastUsed = time.Now()
}

func (s *cliSession) poll(offset int) (lines []string, done bool, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if offset < len(s.lines) {
		lines = s.lines[offset:]
	}
	return lines, s.done, s.exitErr
}

// NewService creates a new CLI service with background session cleanup.
func New() *CliService {
	svc := &CliService{
		sessions: make(map[string]*cliSession),
	}
	go svc.cleanupLoop()
	return svc
}

// NewWithResolver creates a CliService with a custom binary resolver. Use this
// in tests to simulate a missing CLI without touching the system PATH.
func NewWithResolver(fn func(name string) (string, error)) *CliService {
	svc := &CliService{
		sessions:   make(map[string]*cliSession),
		resolveCLI: fn,
	}
	go svc.cleanupLoop()
	return svc
}

// lookupCLI resolves the named binary using the configured resolver, or falls
// back to util.ResolveCLI when none is set.
func (s *CliService) lookupCLI(name string) (string, error) {
	if s.resolveCLI != nil {
		return s.resolveCLI(name)
	}
	return util.ResolveCLI(name)
}

func (s *CliService) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		cutoff := time.Now().Add(-10 * time.Minute)
		for id, sess := range s.sessions {
			sess.mu.Lock()
			if sess.done && sess.lastUsed.Before(cutoff) {
				delete(s.sessions, id)
			}
			sess.mu.Unlock()
		}
		s.mu.Unlock()
	}
}

// ─── Request / Response types ───────────────────────────────────────────────

// StartClaudeRequest describes a claude CLI invocation.
type StartClaudeRequest struct {
	// WorkDir is the repository directory to run claude in.
	WorkDir string `json:"workDir"`
	// Prompt is the user's prompt / task description.
	Prompt string `json:"prompt"`
	// Model e.g. "claude-opus-4-6", "claude-sonnet-4-6", "claude-haiku-4-5"
	Model string `json:"model"`
	// ThinkingDepth: "", "think", "think harder", "ultrathink"
	ThinkingDepth string `json:"thinkingDepth"`
	// Mode: "agent" or "plan"
	Mode string `json:"mode"`
	// PermissionMode currently only supports the default guarded mode.
	PermissionMode string `json:"permissionMode"`
	// AdditionalDirs grants Claude access to paths outside WorkDir when needed.
	AdditionalDirs []string `json:"additionalDirs"`
	// EnvOverrides sets additional environment variables for the claude process.
	// These are applied on top of the current process environment.
	// Use this instead of --model to bypass CLI argument normalization (e.g. 4-6 → 4.6).
	EnvOverrides map[string]string `json:"envOverrides,omitempty"`
}

// StartClaudeResponse holds the session ID for output polling.
type StartClaudeResponse struct {
	SessionID string `json:"sessionId"`
}

// PollOutputRequest specifies which session and line offset to read from.
type PollOutputRequest struct {
	SessionID string `json:"sessionId"`
	Offset    int    `json:"offset"`
}

// PollOutputResponse contains new lines and completion state.
type PollOutputResponse struct {
	Lines  []string `json:"lines"`
	Done   bool     `json:"done"`
	ErrMsg string   `json:"errMsg"`
}

// SkillItem represents a single skill entry.
type SkillItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ─── Public methods ──────────────────────────────────────────────────────────

// CheckCLI returns the resolved path to the claude binary, or an error.
func (s *CliService) CheckCLI() (string, error) {
	path, err := s.lookupCLI("claude")
	if err != nil {
		return "", fmt.Errorf(errs.MsgClaudeCliMissing)
	}
	return path, nil
}

// StartClaude launches a claude CLI session and returns a session ID for polling.
func (s *CliService) StartClaude(req StartClaudeRequest) (*StartClaudeResponse, error) {
	if strings.TrimSpace(req.WorkDir) == "" {
		return nil, fmt.Errorf(errs.MsgWorkDirRequired)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, fmt.Errorf(errs.MsgPromptRequired)
	}

	claudePath, err := s.lookupCLI("claude")
	if err != nil {
		return nil, fmt.Errorf(errs.MsgClaudeCliMissing)
	}

	args, err := buildClaudeArgs(req)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)

	cmd := exec.CommandContext(ctx, claudePath, args...)
	cmd.Dir = req.WorkDir
	if len(req.EnvOverrides) > 0 {
		cmd.Env = applyEnvOverrides(os.Environ(), req.EnvOverrides)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf(errs.FmtStdoutPipeFail, err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf(errs.FmtStderrPipeFail, err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf(errs.FmtClaudeStartClassicFail, err)
	}

	sessionID := uuid.New().String()
	sess := &cliSession{
		cancel:   cancel,
		lastUsed: time.Now(),
	}

	s.mu.Lock()
	s.sessions[sessionID] = sess
	s.mu.Unlock()

	// Get the Wails application instance for event emission.
	app := application.Get()

	// Stream stdout and stderr concurrently
	var wg sync.WaitGroup
	wg.Add(2)

	streamReader := func(r io.Reader, prefix string) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := scanner.Text()
			if prefix != "" {
				line = prefix + line
			}
			sess.append(line)
			// Emit each line as a real-time event so the frontend can display
			// output without polling.
			app.Event.Emit("cli:line:"+sessionID, line)
		}
	}

	go streamReader(stdoutPipe, "")
	go streamReader(stderrPipe, "")

	go func() {
		wg.Wait()
		waitErr := cmd.Wait()
		cancel()
		var errMsg string
		if waitErr != nil {
			if ctx.Err() == context.DeadlineExceeded {
				errMsg = "执行超时（10 分钟）"
			} else {
				errMsg = waitErr.Error()
			}
		}
		// Emit done event before marking session finished so listeners receive
		// the terminal signal.
		app.Event.Emit("cli:done:"+sessionID, errMsg)
		sess.finish(errMsg)
	}()

	return &StartClaudeResponse{SessionID: sessionID}, nil
}

// PollOutput returns new output lines since the given offset.
func (s *CliService) PollOutput(req PollOutputRequest) (*PollOutputResponse, error) {
	s.mu.Lock()
	sess, ok := s.sessions[req.SessionID]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf(errs.FmtSessionNotFound, req.SessionID)
	}

	lines, done, errMsg := sess.poll(req.Offset)
	if lines == nil {
		lines = []string{}
	}
	return &PollOutputResponse{
		Lines:  lines,
		Done:   done,
		ErrMsg: errMsg,
	}, nil
}

// CancelSession terminates a running claude session.
func (s *CliService) CancelSession(sessionID string) error {
	s.mu.Lock()
	sess, ok := s.sessions[sessionID]
	s.mu.Unlock()
	if !ok {
		return nil // already gone or never existed — treat as success
	}
	sess.cancel()
	return nil
}

// ListSkills scans ~/.claude/skills/, reads SKILL.md frontmatter from each
// subdirectory, and returns a sorted list of SkillItem.
func (s *CliService) ListSkills() ([]SkillItem, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf(errs.FmtUserDirFail, err)
	}
	skillsDir := filepath.Join(home, ".claude", "skills")

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []SkillItem{}, nil
		}
		return nil, fmt.Errorf(errs.FmtReadSkillDirFail, err)
	}

	var skills []SkillItem
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillMD := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
		data, err := os.ReadFile(skillMD)
		if err != nil {
			continue
		}
		name, desc := parseSkillFrontmatter(string(data), entry.Name())
		skills = append(skills, SkillItem{Name: name, Description: desc})
	}

	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})
	return skills, nil
}

// parseSkillFrontmatter extracts name and description from YAML frontmatter.
// Falls back to dirName for name and empty string for description.
func parseSkillFrontmatter(content, dirName string) (name, description string) {
	name = dirName
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return
	}
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "---" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "name":
			if val != "" {
				name = val
			}
		case "description":
			if val != "" {
				description = val
			}
		}
	}
	return
}

// InstallBuiltinSkills writes all skills bundled with PINRU to ~/.claude/skills/.
// Always overwrites to keep the installed version in sync with the binary.
// Manual dir placeholders ({{MANUAL_DIR}}) in skill content are replaced with
// the platform-appropriate path before writing.
func (s *CliService) InstallBuiltinSkills() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	manualDir := util.PinruManualDir()
	for dirName, content := range builtinSkills {
		skillDir := filepath.Join(home, ".claude", "skills", dirName)
		skillFile := filepath.Join(skillDir, "SKILL.md")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			continue
		}
		resolved := strings.ReplaceAll(content, "{{MANUAL_DIR}}", manualDir)
		_ = os.WriteFile(skillFile, []byte(resolved), 0o644)
	}
}

// InstallBuiltinManuals extracts the bundled execution manuals to the
// platform data directory (~/.pinru/manuals/). Always overwrites to keep
// the installed version in sync with the binary.
func (s *CliService) InstallBuiltinManuals() {
	destDir := util.PinruManualDir()
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return
	}
	entries, err := manualFS.ReadDir("manuals")
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := manualFS.ReadFile("manuals/" + entry.Name())
		if err != nil {
			continue
		}
		_ = os.WriteFile(filepath.Join(destDir, entry.Name()), data, 0o644)
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func validatePermissionMode(mode string) error {
	trimmed := strings.TrimSpace(mode)
	if trimmed == "" || trimmed == "default" {
		return nil
	}
	switch trimmed {
	case "acceptEdits", "auto", "dontAsk", "plan":
		return nil
	case "yolo", "bypassPermissions":
		return nil
	}
	return fmt.Errorf(errs.FmtUnsupportedPermission, trimmed)
}

func buildClaudeArgs(req StartClaudeRequest) ([]string, error) {
	// Build the final prompt with thinking depth prefix and mode annotation
	finalPrompt := buildPrompt(req.Prompt, req.ThinkingDepth, req.Mode)
	if err := validatePermissionMode(req.PermissionMode); err != nil {
		return nil, err
	}

	args := []string{"-p", finalPrompt}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}

	permissionMode := normalizePermissionMode(req.PermissionMode)
	if permissionMode != "" {
		args = append(args, "--permission-mode", permissionMode)
	}

	if shouldSkipPermissions(req.PermissionMode) {
		args = append(args, "--dangerously-skip-permissions")
	}

	additionalDirs := uniqueNonEmptyStrings(req.AdditionalDirs)
	if len(additionalDirs) > 0 {
		args = append(args, "--add-dir")
		args = append(args, additionalDirs...)
	}

	if req.Mode == "plan" {
		// Plan mode: restrict to read-only tools so claude only plans, doesn't execute.
		args = append(args, "--allowedTools", "Read,Glob,Grep,WebFetch,WebSearch")
	}

	return args, nil
}

func normalizePermissionMode(mode string) string {
	trimmed := strings.TrimSpace(mode)
	switch trimmed {
	case "", "default":
		return ""
	case "yolo":
		return "bypassPermissions"
	default:
		return trimmed
	}
}

func shouldSkipPermissions(mode string) bool {
	trimmed := strings.TrimSpace(mode)
	return trimmed == "" || trimmed == "default" || trimmed == "yolo" || trimmed == "bypassPermissions"
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))

	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}

	return result
}

// applyEnvOverrides merges overrides into base environment slice.
// Each entry in the returned slice has the form "KEY=VALUE".
// Override keys replace any existing entries for the same key.
func applyEnvOverrides(base []string, overrides map[string]string) []string {
	// Build a set of keys to override so we can skip duplicates from base.
	skip := make(map[string]struct{}, len(overrides))
	for k := range overrides {
		skip[strings.ToUpper(k)] = struct{}{}
	}
	result := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key := entry
		if idx := strings.IndexByte(entry, '='); idx >= 0 {
			key = entry[:idx]
		}
		if _, shouldOverride := skip[strings.ToUpper(key)]; !shouldOverride {
			result = append(result, entry)
		}
	}
	for k, v := range overrides {
		result = append(result, k+"="+v)
	}
	return result
}

// ─── Codex Review ────────────────────────────────────────────────────────────

type CodexReviewIssue struct {
	Title        string `json:"title"`
	IssueType    string `json:"issueType"`
	ReviewNotes  string `json:"reviewNotes"`
	NextPrompt   string `json:"nextPrompt"`
	KeyLocations string `json:"keyLocations"`
}

// CodexReviewResult is the structured output from the pg-code review skill.
type CodexReviewResult struct {
	IsCompleted        bool               `json:"isCompleted"`
	IsSatisfied        bool               `json:"isSatisfied"`
	ProjectType        string             `json:"projectType"`
	ChangeScope        string             `json:"changeScope"`
	ReviewNotes        string             `json:"reviewNotes"`
	NextPrompt         string             `json:"nextPrompt"`
	NextPromptTaskType string             `json:"nextPromptTaskType"`
	KeyLocations       string             `json:"keyLocations"`
	Issues             []CodexReviewIssue `json:"issues"`
}

type CodexReviewRequest struct {
	LocalPath         string `json:"localPath"`
	OriginalPrompt    string `json:"originalPrompt"`
	CurrentPrompt     string `json:"currentPrompt"`
	ParentReviewNotes string `json:"parentReviewNotes"`
	IssueType         string `json:"issueType"`
	IssueTitle        string `json:"issueTitle"`
	ModelName         string `json:"modelName"`
}

type pgCodeContextEnvelope struct {
	BaseDir  string                 `json:"base_dir"`
	Projects []pgCodeProjectContext `json:"projects"`
}

type pgCodeProjectContext struct {
	InputPath      string               `json:"input_path"`
	ResolvedPath   string               `json:"resolved_path"`
	Exists         bool                 `json:"exists"`
	ProjectIDGuess string               `json:"project_id_guess"`
	Git            pgCodeGitContext     `json:"git"`
	RecentFiles    []pgCodeRecentFile   `json:"recent_files"`
	Summary        pgCodeProjectSummary `json:"summary"`
}

type pgCodeGitContext struct {
	InGit           bool     `json:"in_git"`
	RepoRoot        *string  `json:"repo_root"`
	StatusLines     []string `json:"status_lines"`
	ChangedFiles    []string `json:"changed_files"`
	ChangedFilesRaw []string `json:"changed_files_repo_relative"`
}

type pgCodeRecentFile struct {
	Path         string `json:"path"`
	RelativePath string `json:"relative_path"`
	MTime        string `json:"mtime"`
}

type pgCodeProjectSummary struct {
	TopLevelEntries []string       `json:"top_level_entries"`
	Extensions      map[string]int `json:"extensions"`
}

// RunCodexReview executes the codex pg-code skill non-interactively on the given
// localPath, streaming each output line to onLine (may be nil), and returns the
// structured review result parsed from the --output-schema JSON file.
func (s *CliService) RunCodexReview(ctx context.Context, req CodexReviewRequest, onLine func(string)) (*CodexReviewResult, error) {
	codexPath, err := s.lookupCLI("codex")
	if err != nil {
		return nil, fmt.Errorf(errs.MsgCodexCliMissing)
	}
	localPath := strings.TrimSpace(req.LocalPath)
	if localPath == "" {
		return nil, fmt.Errorf(errs.MsgLocalPathRequired)
	}
	if strings.TrimSpace(req.OriginalPrompt) == "" && strings.TrimSpace(req.CurrentPrompt) == "" {
		return nil, fmt.Errorf(errs.MsgReviewPromptMissing)
	}

	reviewContext, ctxErr := s.collectPgCodeReviewContext(ctx, localPath)
	if ctxErr != nil {
		slog.Warn("collectPgCodeReviewContext failed", "localPath", localPath, "error", ctxErr)
	}
	reviewPrompt := buildCodexReviewPrompt(req, reviewContext)
	if runtime.GOOS == "windows" {
		reviewPrompt = compactPromptForWindowsCommandLine(reviewPrompt)
	}

	// Write bundled schema to a temp file.
	schemaFile, err := os.CreateTemp("", "pinru-review-schema-*.json")
	if err != nil {
		return nil, fmt.Errorf(errs.FmtSchemaTempFileFail, err)
	}
	schemaPath := schemaFile.Name()
	defer os.Remove(schemaPath)
	if _, err := schemaFile.Write(pgCodeReviewSchema); err != nil {
		schemaFile.Close()
		return nil, fmt.Errorf(errs.FmtWriteSchemaFail, err)
	}
	schemaFile.Close()

	// Temp file for the last-message output.
	outFile, err := os.CreateTemp("", "pinru-review-out-*.json")
	if err != nil {
		return nil, fmt.Errorf(errs.FmtOutputTempFileFail, err)
	}
	outPath := outFile.Name()
	outFile.Close()
	defer os.Remove(outPath)

	args := []string{
		"exec", reviewPrompt,
		"-C", localPath,
		"--dangerously-bypass-approvals-and-sandbox",
		"--output-schema", schemaPath,
		"-o", outPath,
		"--ephemeral",
	}

	cmd := exec.CommandContext(ctx, codexPath, args...)
	cmd.Dir = localPath
	cmd.Env = applyEnvOverrides(os.Environ(), map[string]string{
		"PINRU_CODEX_REVIEW_OUTPUT_PATH": outPath,
	})

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf(errs.FmtStdoutPipeWrap, err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf(errs.FmtStderrPipeWrap, err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf(errs.FmtCodexStartFail, err)
	}

	// Stream stdout and stderr, forwarding each line to the caller.
	var wg sync.WaitGroup
	wg.Add(2)
	var recentOutputMu sync.Mutex
	recentOutput := make([]string, 0, 8)
	streamPipe := func(r io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := scanner.Text()
			appendRecentCodexOutput(&recentOutputMu, &recentOutput, line)
			if onLine != nil {
				onLine(line)
			}
		}
	}
	go streamPipe(stdoutPipe)
	go streamPipe(stderrPipe)
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if summary := formatRecentCodexOutput(recentOutput); summary != "" {
			return nil, fmt.Errorf(errs.FmtCodexRunFailWithSummary, err, summary)
		}
		return nil, fmt.Errorf(errs.FmtCodexRunFail, err)
	}

	// Parse the structured output file.
	data, err := os.ReadFile(outPath)
	if err != nil {
		return nil, fmt.Errorf(errs.FmtCodexReadOutputFail, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf(errs.MsgCodexNoStructuredOutput)
	}

	var result CodexReviewResult
	if err := json.Unmarshal(data, &result); err != nil {
		slog.Error("codex 输出 JSON 解析失败", "err", err, "raw", string(data))
		return nil, fmt.Errorf(errs.FmtCodexParseJSONFail, err)
	}
	applyCodexReviewEvidenceGuards(localPath, reviewContext, &result)
	return &result, nil
}

func appendRecentCodexOutput(mu *sync.Mutex, lines *[]string, line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if len(*lines) == 8 {
		copy((*lines)[0:], (*lines)[1:])
		*lines = (*lines)[:7]
	}
	*lines = append(*lines, trimmed)
}

func formatRecentCodexOutput(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, " | ")
}

func (s *CliService) reviewContextScriptPath() string {
	if strings.TrimSpace(s.reviewContextPath) != "" {
		return s.reviewContextPath
	}
	candidates := defaultPgCodeContextScriptPaths()
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

func (s *CliService) collectPgCodeReviewContext(ctx context.Context, localPath string) (*pgCodeProjectContext, error) {
	scriptPath := strings.TrimSpace(s.reviewContextScriptPath())
	if scriptPath == "" {
		return collectNativePgCodeReviewContext(ctx, localPath)
	}
	if _, err := os.Stat(scriptPath); err != nil {
		slog.Warn("pg-code context script unavailable, using native collector", "scriptPath", scriptPath, "error", err)
		return collectNativePgCodeReviewContext(ctx, localPath)
	}

	pythonPath, err := s.lookupCLI("python3")
	if err != nil {
		slog.Warn("python3 unavailable for pg-code context script, using native collector", "error", err)
		return collectNativePgCodeReviewContext(ctx, localPath)
	}

	cmd := exec.CommandContext(ctx, pythonPath, scriptPath, localPath)
	cmd.Dir = localPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		trimmed := strings.TrimSpace(string(output))
		if trimmed != "" {
			slog.Warn("pg-code context script failed, using native collector", "error", err, "output", trimmed)
			return collectNativePgCodeReviewContext(ctx, localPath)
		}
		slog.Warn("pg-code context script failed, using native collector", "error", err)
		return collectNativePgCodeReviewContext(ctx, localPath)
	}

	var envelope pgCodeContextEnvelope
	if err := json.Unmarshal(output, &envelope); err != nil {
		slog.Warn("pg-code context script output invalid, using native collector", "error", err)
		return collectNativePgCodeReviewContext(ctx, localPath)
	}
	if len(envelope.Projects) == 0 {
		slog.Warn("pg-code context script returned no projects, using native collector", "scriptPath", scriptPath)
		return collectNativePgCodeReviewContext(ctx, localPath)
	}
	return &envelope.Projects[0], nil
}

var nativeContextIgnoredDirs = map[string]struct{}{
	".git": {}, ".hg": {}, ".svn": {}, "node_modules": {}, "dist": {}, "build": {},
	"coverage": {}, ".next": {}, ".nuxt": {}, ".idea": {}, ".vscode": {}, "__pycache__": {},
}

var nativeContextIgnoredSuffixes = map[string]struct{}{
	".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".webp": {}, ".svg": {},
	".ico": {}, ".mp3": {}, ".wav": {}, ".ogg": {}, ".mp4": {}, ".mov": {},
	".pdf": {}, ".zip": {}, ".tar": {}, ".gz": {}, ".lock": {},
}

var nativeContextIgnoredNames = map[string]struct{}{
	".DS_Store": {},
}

type nativeCommandResult struct {
	code   int
	stdout string
	stderr string
}

func collectNativePgCodeReviewContext(ctx context.Context, localPath string) (*pgCodeProjectContext, error) {
	inputPath := strings.TrimSpace(localPath)
	if inputPath == "" {
		return nil, nil
	}

	resolvedPath, err := filepath.Abs(inputPath)
	if err != nil {
		resolvedPath = filepath.Clean(inputPath)
	}
	if realPath, err := filepath.EvalSymlinks(resolvedPath); err == nil {
		resolvedPath = realPath
	}

	project := &pgCodeProjectContext{
		InputPath:      inputPath,
		ResolvedPath:   resolvedPath,
		Exists:         false,
		ProjectIDGuess: guessNativeProjectID(resolvedPath),
		Git: pgCodeGitContext{
			InGit:           false,
			RepoRoot:        nil,
			StatusLines:     []string{},
			ChangedFiles:    []string{},
			ChangedFilesRaw: []string{},
		},
		RecentFiles: []pgCodeRecentFile{},
		Summary: pgCodeProjectSummary{
			TopLevelEntries: []string{},
			Extensions:      map[string]int{},
		},
	}

	info, statErr := os.Stat(resolvedPath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return project, nil
		}
		return project, statErr
	}
	project.Exists = true

	root := resolvedPath
	if !info.IsDir() {
		root = filepath.Dir(resolvedPath)
	}
	project.Git = collectNativeGitContext(ctx, root, 40)
	project.RecentFiles = collectNativeRecentFiles(ctx, root, 12)
	project.Summary = summarizeNativeProjectFiles(root, project.Git.ChangedFiles, project.RecentFiles)
	return project, nil
}

func runNativeCommand(ctx context.Context, args ...string) nativeCommandResult {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		code = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		}
	}
	return nativeCommandResult{
		code:   code,
		stdout: strings.TrimRight(stdout.String(), "\n"),
		stderr: strings.TrimRight(stderr.String(), "\n"),
	}
}

func collectNativeGitContext(ctx context.Context, scope string, changedLimit int) pgCodeGitContext {
	context := pgCodeGitContext{
		InGit:           false,
		RepoRoot:        nil,
		StatusLines:     []string{},
		ChangedFiles:    []string{},
		ChangedFilesRaw: []string{},
	}
	if ctx.Err() != nil {
		return context
	}

	rootResult := runNativeCommand(ctx, "git", "-C", scope, "rev-parse", "--show-toplevel")
	if rootResult.code != 0 || strings.TrimSpace(rootResult.stdout) == "" {
		return context
	}

	repoRoot := strings.TrimSpace(rootResult.stdout)
	if absRoot, err := filepath.Abs(repoRoot); err == nil {
		repoRoot = absRoot
	}
	context.InGit = true
	context.RepoRoot = &repoRoot

	relScope, err := filepath.Rel(repoRoot, scope)
	if err != nil || relScope == "" {
		relScope = "."
	}
	relScope = filepath.ToSlash(relScope)

	statusResult := runNativeCommand(ctx, "git", "-C", repoRoot, "status", "--porcelain", "--untracked-files=all", "--", relScope)
	if statusResult.code == 0 {
		context.StatusLines = limitStrings(nonEmptyLines(statusResult.stdout), changedLimit)
	}

	changedRaw := make([]string, 0, len(context.StatusLines))
	for _, line := range context.StatusLines {
		if len(line) < 4 {
			continue
		}
		changedRaw = append(changedRaw, normalizeNativeStatusPath(line[3:]))
	}
	if len(changedRaw) == 0 {
		diffResult := runNativeCommand(ctx, "git", "-C", repoRoot, "diff", "--name-only", "HEAD", "--", relScope)
		if diffResult.code == 0 {
			changedRaw = nonEmptyLines(diffResult.stdout)
		}
	}

	changedRaw = limitStrings(uniqueStrings(changedRaw), changedLimit)
	context.ChangedFilesRaw = changedRaw
	context.ChangedFiles = repoRelativeToProjectRelative(repoRoot, scope, changedRaw)
	return context
}

func normalizeNativeStatusPath(raw string) string {
	if strings.Contains(raw, " -> ") {
		parts := strings.Split(raw, " -> ")
		raw = parts[len(parts)-1]
	}
	return strings.TrimSpace(raw)
}

func repoRelativeToProjectRelative(repoRoot, scope string, repoRelativePaths []string) []string {
	result := make([]string, 0, len(repoRelativePaths))
	for _, repoRelative := range repoRelativePaths {
		absolute := filepath.Join(repoRoot, filepath.FromSlash(repoRelative))
		if rel, err := filepath.Rel(scope, absolute); err == nil && !strings.HasPrefix(rel, "..") {
			result = append(result, filepath.ToSlash(rel))
			continue
		}
		result = append(result, filepath.ToSlash(repoRelative))
	}
	return result
}

func collectNativeRecentFiles(ctx context.Context, root string, limit int) []pgCodeRecentFile {
	type fileEntry struct {
		mtime time.Time
		path  string
	}
	entries := make([]fileEntry, 0, limit)
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || ctx.Err() != nil {
			return nil
		}
		name := entry.Name()
		if entry.IsDir() {
			if _, ignored := nativeContextIgnoredDirs[name]; ignored {
				return filepath.SkipDir
			}
			return nil
		}
		if _, ignored := nativeContextIgnoredNames[name]; ignored {
			return nil
		}
		if _, ignored := nativeContextIgnoredSuffixes[strings.ToLower(filepath.Ext(name))]; ignored {
			return nil
		}
		info, statErr := entry.Info()
		if statErr != nil {
			return nil
		}
		entries = append(entries, fileEntry{mtime: info.ModTime(), path: path})
		return nil
	})

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].mtime.Equal(entries[j].mtime) {
			return entries[i].path < entries[j].path
		}
		return entries[i].mtime.After(entries[j].mtime)
	})

	if len(entries) > limit {
		entries = entries[:limit]
	}
	result := make([]pgCodeRecentFile, 0, len(entries))
	for _, entry := range entries {
		rel, err := filepath.Rel(root, entry.path)
		if err != nil {
			rel = entry.path
		}
		result = append(result, pgCodeRecentFile{
			Path:         entry.path,
			RelativePath: filepath.ToSlash(rel),
			MTime:        entry.mtime.Format("2006-01-02T15:04:05"),
		})
	}
	return result
}

func summarizeNativeProjectFiles(root string, changedFiles []string, recentFiles []pgCodeRecentFile) pgCodeProjectSummary {
	sourcePaths := make([]string, 0, len(changedFiles)+len(recentFiles))
	for _, rel := range changedFiles {
		sourcePaths = append(sourcePaths, filepath.Join(root, filepath.FromSlash(rel)))
	}
	if len(sourcePaths) == 0 {
		for _, file := range recentFiles {
			sourcePaths = append(sourcePaths, filepath.Join(root, filepath.FromSlash(file.RelativePath)))
		}
	}

	topCounts := make(map[string]int)
	extCounts := make(map[string]int)
	for _, path := range sourcePaths {
		rel, err := filepath.Rel(root, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) > 0 && parts[0] != "" {
			topCounts[parts[0]]++
		} else {
			topCounts["."]++
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == "" {
			ext = "<no-ext>"
		}
		extCounts[ext]++
	}

	return pgCodeProjectSummary{
		TopLevelEntries: topNamesByCount(topCounts, 6),
		Extensions:      topMapByCount(extCounts, 8),
	}
}

func guessNativeProjectID(path string) string {
	for _, part := range pathPartsFromLeaf(path, 4) {
		if strings.Contains(part, "label-") {
			return part
		}
	}
	base := filepath.Base(path)
	if base == "." || base == string(filepath.Separator) {
		return path
	}
	return base
}

func pathPartsFromLeaf(path string, limit int) []string {
	parts := make([]string, 0, limit)
	current := filepath.Clean(path)
	for len(parts) < limit {
		base := filepath.Base(current)
		if base == "." || base == string(filepath.Separator) || base == "" {
			break
		}
		parts = append(parts, base)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return parts
}

func nonEmptyLines(text string) []string {
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func limitStrings(values []string, limit int) []string {
	if limit >= 0 && len(values) > limit {
		return values[:limit]
	}
	return values
}

func topNamesByCount(counts map[string]int, limit int) []string {
	items := sortCountKeys(counts)
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func topMapByCount(counts map[string]int, limit int) map[string]int {
	items := sortCountKeys(counts)
	if len(items) > limit {
		items = items[:limit]
	}
	result := make(map[string]int, len(items))
	for _, key := range items {
		result[key] = counts[key]
	}
	return result
}

func sortCountKeys(counts map[string]int) []string {
	items := make([]string, 0, len(counts))
	for key := range counts {
		items = append(items, key)
	}
	sort.Slice(items, func(i, j int) bool {
		if counts[items[i]] == counts[items[j]] {
			return items[i] < items[j]
		}
		return counts[items[i]] > counts[items[j]]
	})
	return items
}

func buildCodexReviewPrompt(req CodexReviewRequest, project *pgCodeProjectContext) string {
	var parts []string
	parts = append(parts, "/pg-code")
	parts = append(parts, strings.TrimSpace(`
补充规则：
1. 【严格限制】只能基于任务提示词、git 变更、最近更新文件以及你实际读取过的文件下结论。允许主动读取和评审的仓库文件仍限于 git 变更文件（git status / git diff 列出的文件）以及最近更新文件。
2. 严禁猜测运行效果、页面视觉、接口返回、测试结果或用户体验。
3. keyLocations 只能填写 git 变更文件或最近更新文件中 1 到 3 个你实际核验过的代码位置，写不出时可留空。
4. isCompleted 和 isSatisfied 必须分开判断：核心交付物出现、主流程大体落地时，isCompleted 可填 true；只有主要求覆盖接近 90 分、没有明确主链路缺口、关键边界不影响验收时，isSatisfied 才能填 true。80% 左右只能算“完成但不满意”，不能直接满意通过。
5. 找不到任务提示词或有效改动时，reviewNotes 注明”依据不足”，isCompleted 和 isSatisfied 均填 false。
6. projectType 和 changeScope 按最符合实际情况的选项填写。
7. 任务提示词以“当前复核节点上下文”里的 original_prompt/current_prompt 为唯一来源，只把其中明确写出的要求作为验收标准；不要再去读取本地提示词文件，也不要把未写明的扩展点、常识性联想、顺手优化项记为未完成或不满意。parent_review_notes 仅作辅助上下文，不能替代任务提示词本身。
8. 当 isCompleted=false 或 isSatisfied=false 时，reviewNotes 必须回指 original_prompt/current_prompt 中对应的具体句子、短语或明确要求；若拆分到 issues，则每条 issues[*].reviewNotes 也必须分别回指对应 prompt 语句。回指不到的内容不能作为主缺口，不得据此判定未完成或不满意。
9. nextPrompt 只能围绕主缺口补充最小修复指令，必须与已回指的 prompt 要求直接对应，不得扩展额外需求；若拆分到 issues，则每条 issues[*].nextPrompt 也遵守同样规则。
10. 当本轮发现多个独立问题时，必须通过 issues 数组分别列出；不要把多个问题揉成一条。
11. nextPromptTaskType 根据 nextPrompt 的任务性质填写，只能在“Bug修复、Feature迭代、0-1代码生成、代码理解、代码重构、工程化、代码测试、未归类”中选择；满意且 nextPrompt 为“无”时填“未归类”。
12. issues[*].issueType 默认填“Bug修复”，除非证据明确表明是其他类型。
13. 若本轮已通过，issues 返回空数组，但 reviewNotes 不能只填“无”；必须用一两句话说明已经核验哪些核心要求和关键代码位置，作为通过依据。
`))

	reviewInput := map[string]string{
		"issue_title":         strings.TrimSpace(req.IssueTitle),
		"issue_type":          strings.TrimSpace(req.IssueType),
		"model_name":          strings.TrimSpace(req.ModelName),
		"original_prompt":     strings.TrimSpace(req.OriginalPrompt),
		"current_prompt":      strings.TrimSpace(req.CurrentPrompt),
		"parent_review_notes": strings.TrimSpace(req.ParentReviewNotes),
	}
	if contextJSON, err := json.MarshalIndent(reviewInput, "", "  "); err == nil {
		parts = append(parts, "当前复核节点上下文如下，请明确区分“原始任务提示词”“当前节点提示词”“父节点不满意结论”：\n"+string(contextJSON))
	}

	if project != nil {
		contextJSON, err := json.MarshalIndent(project, "", "  ")
		if err == nil {
			parts = append(parts, "下面是预采集到的项目上下文，请优先据此取证，不要忽略证据缺口：\n"+string(contextJSON))
		}
	}

	return strings.Join(parts, "\n\n")
}

func compactPromptForWindowsCommandLine(prompt string) string {
	replacer := strings.NewReplacer("\r\n", "\n", "\r", "\n")
	lines := strings.Split(replacer.Replace(prompt), "\n")
	compacted := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			compacted = append(compacted, trimmed)
		}
	}
	return strings.Join(compacted, " ")
}

func applyCodexReviewEvidenceGuards(localPath string, project *pgCodeProjectContext, result *CodexReviewResult) {
	if result == nil {
		return
	}

	// Hard guards: project must exist and have some verifiable changes.
	var hardReasons []string
	if project == nil {
		hardReasons = append(hardReasons, "未采集到复审上下文")
	} else {
		if !project.Exists {
			hardReasons = append(hardReasons, "项目目录不存在")
		}
		if len(project.Git.ChangedFiles) == 0 && len(project.RecentFiles) == 0 {
			hardReasons = append(hardReasons, "缺少可核验的改动或最近文件")
		}
	}

	if len(hardReasons) > 0 {
		result.IsCompleted = false
		result.IsSatisfied = false
		guardNote := "依据不足：" + strings.Join(hardReasons, "；")
		if note := strings.TrimSpace(result.ReviewNotes); note == "" || note == "无" {
			result.ReviewNotes = guardNote
		} else if !strings.Contains(note, "依据不足") {
			result.ReviewNotes = note + "；" + guardNote
		}
		if prompt := strings.TrimSpace(result.NextPrompt); prompt == "" || prompt == "无" {
			result.NextPrompt = "先补齐可核验的提示词和关键代码位置，再重新复审。"
		}
		return
	}

	if result.IsCompleted && result.IsSatisfied && isEmptyPassingReviewNote(result.ReviewNotes) {
		result.IsSatisfied = false
		result.ReviewNotes = "通过依据不足：isSatisfied=true 时 reviewNotes 不能只填“无”，需要说明已核验的核心要求和关键代码证据。"
		if prompt := strings.TrimSpace(result.NextPrompt); prompt == "" || prompt == "无" {
			result.NextPrompt = "请补充复审通过依据，明确说明已核验哪些核心要求和关键代码位置；如存在主链路缺口，则按实际缺口修复。"
		}
		if normalized := strings.TrimSpace(result.NextPromptTaskType); normalized == "" || normalized == "未归类" {
			result.NextPromptTaskType = "未归类"
		}
	}

	// Soft guard: invalid key locations are noted but do not override the
	// AI's pass/fail judgment.
	if countValidKeyLocations(localPath, result.KeyLocations) == 0 && strings.TrimSpace(result.KeyLocations) != "" {
		note := "注：关键代码位置格式无效"
		if existing := strings.TrimSpace(result.ReviewNotes); existing == "" || existing == "无" {
			result.ReviewNotes = note
		} else {
			result.ReviewNotes = existing + "；" + note
		}
	}
}

func isEmptyPassingReviewNote(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed == "" || trimmed == "无" || strings.EqualFold(trimmed, "none") || strings.EqualFold(trimmed, "n/a")
}

func countValidKeyLocations(localPath, raw string) int {
	entries := splitKeyLocations(raw)
	valid := 0
	for _, entry := range entries {
		if isValidKeyLocation(localPath, entry) {
			valid++
		}
	}
	return valid
}

func splitKeyLocations(raw string) []string {
	replacer := strings.NewReplacer("；", ";", "，", ";", ",", ";")
	normalized := replacer.Replace(raw)
	parts := strings.Split(normalized, ";")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func isValidKeyLocation(localPath, entry string) bool {
	idx := strings.LastIndex(entry, ":")
	if idx <= 0 || idx >= len(entry)-1 {
		return false
	}

	relativePath := strings.TrimSpace(entry[:idx])
	lineText := strings.TrimSpace(entry[idx+1:])
	lineNumber, err := strconv.Atoi(lineText)
	if err != nil || lineNumber <= 0 {
		return false
	}

	filePath := filepath.Join(localPath, relativePath)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return false
	}

	lineCount := bytes.Count(data, []byte{'\n'})
	if len(data) > 0 && data[len(data)-1] != '\n' {
		lineCount++
	}
	return lineCount >= lineNumber
}

func buildPrompt(userPrompt, thinkingDepth, mode string) string {
	var sb strings.Builder

	// Thinking depth prefix
	switch strings.ToLower(thinkingDepth) {
	case "think":
		sb.WriteString("think\n\n")
	case "think harder":
		sb.WriteString("think harder\n\n")
	case "ultrathink":
		sb.WriteString("ultrathink\n\n")
	}

	// Mode annotation
	if mode == "plan" {
		sb.WriteString("【规划模式】仅输出实施计划，不执行任何文件修改或命令，不使用 Write/Edit/Bash 工具。\n\n")
	}

	sb.WriteString(userPrompt)
	return sb.String()
}
