package prompt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	appcli "github.com/blueship581/pinru/app/cli"
	"github.com/blueship581/pinru/internal/errs"
	"github.com/blueship581/pinru/internal/llm"
	internalprompt "github.com/blueship581/pinru/internal/prompt"
	"github.com/blueship581/pinru/internal/store"
	"github.com/blueship581/pinru/internal/util"
)

// Service handles prompt generation and storage for tasks.
type PromptService struct {
	store                   *store.Store
	cliSvc                  *appcli.CliService
	promptGenerator         func(context.Context, string, string, string) (generatedPromptResult, error)
	promptHumanizer         func(context.Context, string, string, string) (string, error)
	duplicateJudge          func(context.Context, string, string, string) (semanticDuplicateDecision, error)
	requirementDocGenerator func(context.Context, string, string, string) (string, error)
}

// NewService creates a new prompt service.
func New(store *store.Store, cliSvc *appcli.CliService) *PromptService {
	return &PromptService{store: store, cliSvc: cliSvc}
}

// GeneratePromptRequest carries parameters for prompt generation.
type GeneratePromptRequest struct {
	TaskID          string   `json:"taskId"`
	ProviderID      *string  `json:"providerId"`
	TaskType        string   `json:"taskType"`
	Scopes          []string `json:"scopes"`
	Constraints     []string `json:"constraints"`
	AdditionalNotes *string  `json:"additionalNotes"`
	ThinkingBudget  string   `json:"thinkingBudget"`
}

// PromptGenerationResult is returned after a successful generation.
type PromptGenerationResult struct {
	PromptText       string `json:"promptText"`
	PromptDifficulty string `json:"promptDifficulty"`
	ProviderName     string `json:"providerName"`
	Model            string `json:"model"`
	Status           string `json:"status"`
}

type GenerateCustomProjectPromptDocumentsRequest struct {
	ProjectID    string   `json:"projectId"`
	ProjectNames []string `json:"projectNames"`
	ProviderID   *string  `json:"providerId"`
}

type CustomProjectPromptDocumentDetail struct {
	ProjectName string `json:"projectName"`
	SourcePath  string `json:"sourcePath"`
	OutputPath  string `json:"outputPath"`
	Content     string `json:"content"`
	Status      string `json:"status"`
	Message     string `json:"message"`
}

type GenerateCustomProjectPromptDocumentsResult struct {
	ProjectID      string                              `json:"projectId"`
	RootPath       string                              `json:"rootPath"`
	ProviderName   string                              `json:"providerName"`
	Model          string                              `json:"model"`
	GeneratedCount int                                 `json:"generatedCount"`
	ErrorCount     int                                 `json:"errorCount"`
	Details        []CustomProjectPromptDocumentDetail `json:"details"`
}

type CustomProjectPromptDocumentProgress struct {
	ProjectName string
	Index       int
	Total       int
	Stage       string
}

type GenerateCustomProjectPromptDocumentsOptions struct {
	OnProgress func(CustomProjectPromptDocumentProgress)
}

type CustomProjectPromptDocumentRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

const defaultPromptGenerationModel = "claude-sonnet-4-6"

func (s *PromptService) GenerateTaskPrompt(req GeneratePromptRequest) (*PromptGenerationResult, error) {
	return s.GenerateTaskPromptWithContext(context.Background(), req)
}

func (s *PromptService) TestLLMProvider(provider store.LLMProvider) (bool, error) {
	resolved, err := s.resolveProviderForTest(provider)
	if err != nil {
		return false, err
	}

	client, err := llm.BuildProvider(llm.Config{
		ID:           resolved.ID,
		Name:         resolved.Name,
		ProviderType: resolved.ProviderType,
		Model:        resolved.Model,
		BaseURL:      resolved.BaseURL,
		APIKey:       resolved.APIKey,
		IsDefault:    resolved.IsDefault,
	})
	if err != nil {
		return false, err
	}

	if err := client.TestConnection(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *PromptService) GenerateTaskPromptWithContext(ctx context.Context, req GeneratePromptRequest) (*PromptGenerationResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(req.TaskID) == "" {
		return nil, errors.New(errs.MsgTaskRequired)
	}
	if strings.TrimSpace(req.TaskType) == "" {
		return nil, errors.New(errs.MsgTaskTypeRequired)
	}

	task, err := s.store.GetTask(req.TaskID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, fmt.Errorf(errs.FmtTaskNotFound, req.TaskID)
	}
	if task.LocalPath == nil {
		return nil, errors.New(errs.MsgTaskMissingWorkDir)
	}

	if _, err := s.cliSvc.CheckCLI(); err != nil {
		return nil, errors.New(errs.MsgClaudeCodeCliNotInstalledInstallGuide)
	}

	selection, err := resolveProviderForPromptGeneration(s.store, req.ProviderID)
	if err != nil {
		return nil, err
	}

	// 查询同题源下已有提示词的兄弟任务（B-35-1 的提示词要在 B-35-2 生成时传入，
	// 用于去重约束）。GitLab 和压缩包来源都通过 GitLabProjectID 聚合，无需区分。
	var siblingPrompts []siblingPrompt
	if task.ProjectConfigID != nil && strings.TrimSpace(*task.ProjectConfigID) != "" {
		siblings, err := s.store.ListSiblingTasksWithPrompt(*task.ProjectConfigID, task.GitLabProjectID, task.ID)
		if err != nil {
			slog.Warn("list sibling prompts failed", "task_id", task.ID, "error", err)
		} else {
			siblingPrompts = collectSiblingPrompts(siblings)
		}
	}
	existingPrompts := collectPromptDedupSources(task, siblingPrompts)

	startedAt := time.Now().Unix()
	if err := s.store.StartTaskPromptGeneration(task.ID, startedAt); err != nil {
		return nil, err
	}

	// Build the [PINRU] skill prompt. Keep generation context small; semantic duplicate
	// checks run after generation against a broader candidate set.
	generationPrompts := promptGenerationContext(existingPrompts)
	workDir := util.NormalizePath(*task.LocalPath)
	projectProfile, err := s.resolveProjectProfile(ctx, workDir)
	if err != nil {
		slog.Warn("project profile cache unavailable, falling back to live repository reading",
			"task_id", task.ID,
			"project", task.ProjectName,
			"error", err,
		)
	}
	skillPrompt := buildSkillPrompt(req, generationPrompts, projectProfile)

	// Execute CLI Agent with one automatic retry on failure
	slog.Info("CLI prompt generation started",
		"project", task.ProjectName,
		"task_type", req.TaskType,
		"model", selection.Model,
		"provider", selection.Name,
		"profile", projectProfile.summaryForLog(),
	)
	cliStart := time.Now()
	generated, err := s.generatePromptWithRetry(ctx, workDir, skillPrompt, selection.Model, 1)
	if err != nil {
		slog.Error("CLI prompt generation failed",
			"project", task.ProjectName,
			"model", selection.Model,
			"elapsed", time.Since(cliStart).Round(time.Millisecond),
			"error", err,
		)
		errMsg := normalizePromptGenerationError(err)
		if failErr := s.store.FailTaskPromptGeneration(task.ID, errMsg, startedAt); failErr != nil {
			return nil, fmt.Errorf(errs.FmtPromptStatusBack, errMsg, failErr)
		}
		return nil, errors.New(errMsg)
	}
	promptText := generated.PromptText
	modelDifficulty := generated.PromptDifficulty
	if duplicate, ok := s.findDuplicatePromptMatch(ctx, workDir, promptText, existingPrompts, selection.Model); ok {
		slog.Warn("generated prompt duplicated existing prompt, regenerating",
			"task_id", task.ID,
			"project", task.ProjectName,
			"model", selection.Model,
			"duplicate_task_id", duplicate.TaskID,
			"duplicate_reason", duplicate.Reason,
			"duplicate_confidence", duplicate.Confidence,
		)
		regeneratePrompt := buildDuplicateRegenerationPrompt(req, existingPrompts, promptText, duplicate, projectProfile)
		generated, err = s.generatePromptWithRetry(ctx, workDir, regeneratePrompt, selection.Model, 1)
		if err != nil {
			slog.Error("CLI prompt regeneration failed",
				"project", task.ProjectName,
				"model", selection.Model,
				"elapsed", time.Since(cliStart).Round(time.Millisecond),
				"error", err,
			)
			errMsg := normalizePromptGenerationError(err)
			if failErr := s.store.FailTaskPromptGeneration(task.ID, errMsg, startedAt); failErr != nil {
				return nil, fmt.Errorf(errs.FmtPromptStatusBack, errMsg, failErr)
			}
			return nil, errors.New(errMsg)
		}
		promptText = generated.PromptText
		modelDifficulty = generated.PromptDifficulty
	}
	slog.Info("CLI prompt generation completed",
		"project", task.ProjectName,
		"model", selection.Model,
		"elapsed", time.Since(cliStart).Round(time.Millisecond),
	)

	rawGeneratedPrompt := strings.TrimSpace(promptText)
	promptText = s.bestEffortPolishPrompt(ctx, workDir, promptText, selection)
	if duplicate, ok := findExactDuplicatePromptMatch(promptText, existingPrompts); ok && !isDuplicatePrompt(rawGeneratedPrompt, existingPrompts) {
		slog.Warn("polished prompt duplicated existing prompt, keeping raw generated prompt",
			"task_id", task.ID,
			"project", task.ProjectName,
			"model", selection.Model,
			"duplicate_task_id", duplicate.TaskID,
		)
		promptText = rawGeneratedPrompt
	}
	promptDifficulty := modelDifficulty
	if promptDifficulty == "" {
		promptDifficulty = estimatePromptDifficulty(req, promptText)
	}

	if err := s.store.CompleteTaskPromptGenerationWithDifficulty(task.ID, promptText, promptDifficulty, startedAt); err != nil {
		return nil, err
	}
	BestEffortSyncTaskPromptArtifact(task, promptText)

	return &PromptGenerationResult{
		PromptText:       promptText,
		PromptDifficulty: promptDifficulty,
		ProviderName:     selection.Name,
		Model:            selection.Model,
		Status:           "PromptReady",
	}, nil
}

func (s *PromptService) SaveTaskPrompt(taskID, promptText string) error {
	if strings.TrimSpace(taskID) == "" {
		return errors.New(errs.MsgTaskRequired)
	}
	if strings.TrimSpace(promptText) == "" {
		return errors.New(errs.MsgPromptContentRequired)
	}

	task, err := LoadTaskForPromptSync(s.store, taskID)
	if err != nil {
		return err
	}

	if err := s.store.UpdateTaskPrompt(taskID, promptText); err != nil {
		return err
	}

	BestEffortSyncTaskPromptArtifact(task, promptText)
	return nil
}

func (s *PromptService) GenerateCustomProjectPromptDocuments(req GenerateCustomProjectPromptDocumentsRequest) (*GenerateCustomProjectPromptDocumentsResult, error) {
	return s.GenerateCustomProjectPromptDocumentsWithContext(context.Background(), req)
}

func (s *PromptService) GenerateCustomProjectPromptDocumentsWithContext(ctx context.Context, req GenerateCustomProjectPromptDocumentsRequest) (*GenerateCustomProjectPromptDocumentsResult, error) {
	return s.GenerateCustomProjectPromptDocumentsWithOptions(ctx, req, nil)
}

func (s *PromptService) GenerateCustomProjectPromptDocumentsWithOptions(
	ctx context.Context,
	req GenerateCustomProjectPromptDocumentsRequest,
	options *GenerateCustomProjectPromptDocumentsOptions,
) (*GenerateCustomProjectPromptDocumentsResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		return nil, errors.New(errs.MsgProjectRequired)
	}
	projectNames := normalizeCustomProjectDocumentNames(req.ProjectNames)
	if len(projectNames) == 0 {
		return nil, errors.New("未选择任何自定义项目")
	}
	if _, err := s.cliSvc.CheckCLI(); err != nil {
		return nil, errors.New(errs.MsgClaudeCodeCliNotInstalledInstallGuide)
	}
	selection, err := resolveProviderForPromptGeneration(s.store, req.ProviderID)
	if err != nil {
		return nil, err
	}

	project, err := s.store.GetProject(projectID)
	if err != nil {
		return nil, err
	}
	if project == nil {
		return nil, fmt.Errorf(errs.FmtStoreProjectNotFound, projectID)
	}

	rootPath, err := s.store.GetConfig("custom_project_root_path")
	if err != nil {
		rootPath = ""
	}
	rootPath = util.NormalizePath(rootPath)
	if rootPath == "" {
		return nil, errors.New("请先在设置中配置自定义项目根目录")
	}
	if err := os.MkdirAll(util.ExpandTilde(rootPath), 0o755); err != nil {
		return nil, err
	}

	items, err := s.store.ListQuestionBankItems(projectID)
	if err != nil {
		return nil, err
	}
	itemByName := make(map[string]store.QuestionBankItem, len(items))
	for _, item := range items {
		if !isCustomQuestionBankItem(item) {
			continue
		}
		itemByName[strings.ToLower(strings.TrimSpace(item.DisplayName))] = item
	}

	result := &GenerateCustomProjectPromptDocumentsResult{
		ProjectID:    project.ID,
		RootPath:     rootPath,
		ProviderName: selection.Name,
		Model:        selection.Model,
		Details:      make([]CustomProjectPromptDocumentDetail, 0, len(projectNames)),
	}

	emitProgress := func(projectName string, index int, stage string) {
		if options != nil && options.OnProgress != nil {
			options.OnProgress(CustomProjectPromptDocumentProgress{
				ProjectName: projectName,
				Index:       index,
				Total:       len(projectNames),
				Stage:       stage,
			})
		}
	}

	for index, projectName := range projectNames {
		progressIndex := index + 1
		emitProgress(projectName, progressIndex, "start")
		detail := CustomProjectPromptDocumentDetail{
			ProjectName: projectName,
			Status:      "error",
		}
		item, ok := itemByName[strings.ToLower(projectName)]
		if !ok {
			detail.Message = "未找到已导入的自定义项目题库记录"
			result.Details = append(result.Details, detail)
			result.ErrorCount++
			emitProgress(projectName, progressIndex, "error")
			continue
		}
		sourcePath := util.NormalizePath(item.SourcePath)
		detail.SourcePath = sourcePath
		outputPath := buildCustomProjectPromptDocumentPath(rootPath, item.DisplayName, time.Now())
		detail.OutputPath = outputPath

		emitProgress(projectName, progressIndex, "generating")
		content, genErr := s.generateCustomProjectPromptDocument(ctx, sourcePath, item.DisplayName, selection.Model)
		if genErr != nil {
			detail.Message = genErr.Error()
			result.Details = append(result.Details, detail)
			result.ErrorCount++
			emitProgress(projectName, progressIndex, "error")
			continue
		}
		emitProgress(projectName, progressIndex, "writing")
		if err := os.WriteFile(util.ExpandTilde(outputPath), []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
			detail.Message = err.Error()
			result.Details = append(result.Details, detail)
			result.ErrorCount++
			emitProgress(projectName, progressIndex, "error")
			continue
		}
		detail.Content = strings.TrimSpace(content)
		detail.Status = "generated"
		detail.Message = "已生成提示词文档"
		result.Details = append(result.Details, detail)
		result.GeneratedCount++
		emitProgress(projectName, progressIndex, "done")
	}

	return result, nil
}

func (s *PromptService) ReadCustomProjectPromptDocument(path string) (*CustomProjectPromptDocumentDetail, error) {
	documentPath := util.NormalizePath(path)
	if strings.TrimSpace(documentPath) == "" {
		return nil, errors.New("提示词文档路径不能为空")
	}
	content, err := os.ReadFile(util.ExpandTilde(documentPath))
	if err != nil {
		return nil, err
	}
	return &CustomProjectPromptDocumentDetail{
		ProjectName: inferCustomProjectNameFromPromptDocumentPath(documentPath),
		OutputPath:  documentPath,
		Content:     strings.TrimSpace(string(content)),
		Status:      "loaded",
		Message:     "已读取提示词文档",
	}, nil
}

func (s *PromptService) SaveCustomProjectPromptDocument(req CustomProjectPromptDocumentRequest) (*CustomProjectPromptDocumentDetail, error) {
	documentPath := util.NormalizePath(req.Path)
	if strings.TrimSpace(documentPath) == "" {
		return nil, errors.New("提示词文档路径不能为空")
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return nil, errors.New("提示词文档内容不能为空")
	}
	if err := os.WriteFile(util.ExpandTilde(documentPath), []byte(content+"\n"), 0o644); err != nil {
		return nil, err
	}
	return &CustomProjectPromptDocumentDetail{
		ProjectName: inferCustomProjectNameFromPromptDocumentPath(documentPath),
		OutputPath:  documentPath,
		Content:     content,
		Status:      "saved",
		Message:     "已保存提示词文档",
	}, nil
}

// ── CLI Agent 执行 ──────────────────────────────────────────────────────────

type generatedPromptResult struct {
	PromptText       string
	PromptDifficulty string
}

func (s *PromptService) generatePromptWithRetry(ctx context.Context, workDir, prompt, model string, maxRetries int) (generatedPromptResult, error) {
	if s.promptGenerator != nil {
		return s.promptGenerator(ctx, workDir, prompt, model)
	}
	return s.executeCliWithRetry(ctx, workDir, prompt, model, maxRetries)
}

func (s *PromptService) runPromptHumanizer(ctx context.Context, workDir, prompt, model string) (string, error) {
	if s.promptHumanizer != nil {
		return s.promptHumanizer(ctx, workDir, prompt, model)
	}
	return "", nil
}

func (s *PromptService) generateCustomProjectPromptDocument(ctx context.Context, workDir, projectName, model string) (string, error) {
	var output string
	var err error
	if s.requirementDocGenerator != nil {
		output, err = s.requirementDocGenerator(ctx, workDir, projectName, model)
	} else {
		projectProfile, profileErr := s.resolveProjectProfile(ctx, workDir)
		if profileErr != nil {
			slog.Warn("custom project profile cache unavailable, falling back to live repository reading",
				"project", projectName,
				"error", profileErr,
			)
		}
		prompt := buildCustomProjectPromptDocumentPrompt(projectName, projectProfile)
		output, err = s.executeCliRaw(ctx, workDir, prompt, model)
	}
	if err != nil {
		return "", err
	}
	content := cleanCustomProjectPromptDocument(output)
	if strings.TrimSpace(content) == "" || !strings.Contains(content, "0-1代码生成") {
		trimmedOutput := strings.TrimSpace(output)
		if trimmedOutput == "" {
			return "", errors.New("模型未返回可写入的提示词文档")
		}
		return "", fmt.Errorf("模型未返回有效的提示词需求文档: %s", trimmedOutput)
	}
	return content, nil
}

func (s *PromptService) bestEffortPolishPrompt(ctx context.Context, workDir, promptText string, selection promptProviderSelection) string {
	original := strings.TrimSpace(promptText)
	if original == "" {
		return original
	}
	if s.promptHumanizer == nil {
		return original
	}

	polishPrompt := buildPolishSkillPrompt(original)
	start := time.Now()
	polished, err := s.runPromptHumanizer(ctx, workDir, polishPrompt, selection.Model)
	if err != nil {
		slog.Warn("prompt polish failed, using original",
			"model", selection.Model,
			"provider", selection.Name,
			"elapsed", time.Since(start).Round(time.Millisecond),
			"error", err,
		)
		return original
	}

	polished = strings.TrimSpace(polished)
	if polished == "" {
		slog.Warn("prompt polish returned empty text, using original",
			"model", selection.Model,
			"provider", selection.Name,
			"elapsed", time.Since(start).Round(time.Millisecond),
		)
		return original
	}

	slog.Info("prompt polish completed",
		"model", selection.Model,
		"provider", selection.Name,
		"elapsed", time.Since(start).Round(time.Millisecond),
		"changed", polished != original,
	)
	return polished
}

// executeCliWithRetry 执行 CLI Agent 生成提示词，失败时自动重试指定次数。
func (s *PromptService) executeCliWithRetry(ctx context.Context, workDir, prompt, model string, maxRetries int) (generatedPromptResult, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			slog.Warn("retrying CLI prompt generation",
				"attempt", attempt,
				"max_retries", maxRetries,
				"last_error", lastErr,
			)
		}
		result, err := s.executeCliPromptGeneration(ctx, workDir, prompt, model)
		if err == nil {
			return result, nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return generatedPromptResult{}, err
		}
		lastErr = err
	}
	return generatedPromptResult{}, fmt.Errorf(errs.FmtPromptRetryFailed, maxRetries, lastErr)
}

// executeCliPromptGeneration 启动一次 CLI Agent 执行并从输出中提取提示词。
func (s *PromptService) executeCliPromptGeneration(ctx context.Context, workDir, prompt, model string) (generatedPromptResult, error) {
	output, err := s.executeCliRaw(ctx, workDir, prompt, model)
	if err != nil {
		return generatedPromptResult{}, err
	}

	result, err := ExtractPromptResultFromCLIOutput(output)
	if err != nil {
		return generatedPromptResult{}, fmt.Errorf(errs.FmtPromptExtractFail, err)
	}

	return generatedPromptResult{
		PromptText:       result.PromptText,
		PromptDifficulty: result.PromptDifficulty,
	}, nil
}

func (s *PromptService) executeCliHumanizer(ctx context.Context, workDir, prompt, model string) (string, error) {
	output, err := s.executeCliRaw(ctx, workDir, prompt, model)
	if err != nil {
		return "", err
	}

	humanizedText, err := ExtractHumanizedTextFromCLIOutput(output)
	if err != nil {
		return "", fmt.Errorf(errs.FmtPolishExtractFail, err)
	}

	return humanizedText, nil
}

func (s *PromptService) executeCliRaw(ctx context.Context, workDir, prompt, model string) (string, error) {
	additionalDirs := cliAdditionalDirs()

	resp, err := s.cliSvc.StartClaude(appcli.StartClaudeRequest{
		WorkDir:        workDir,
		Prompt:         prompt,
		Model:          model,
		PermissionMode: "bypassPermissions",
		AdditionalDirs: additionalDirs,
	})
	if err != nil {
		return "", fmt.Errorf(errs.FmtClaudeStartFail, err)
	}

	output, err := s.waitForCliCompletion(ctx, resp.SessionID)
	if err != nil {
		return "", err
	}
	return output, nil
}

// waitForCliCompletion 同步轮询等待 CLI 执行完成，返回完整输出。
func (s *PromptService) waitForCliCompletion(ctx context.Context, sessionID string) (string, error) {
	var lines []string
	offset := 0

	for {
		select {
		case <-ctx.Done():
			_ = s.cliSvc.CancelSession(sessionID)
			return "", ctx.Err()
		default:
		}

		poll, err := s.cliSvc.PollOutput(appcli.PollOutputRequest{
			SessionID: sessionID,
			Offset:    offset,
		})
		if err != nil {
			return "", fmt.Errorf(errs.FmtCliPollFail, err)
		}

		lines = append(lines, poll.Lines...)
		offset += len(poll.Lines)

		if poll.Done {
			if poll.ErrMsg != "" {
				return "", fmt.Errorf(errs.FmtClaudeRunErr, poll.ErrMsg)
			}
			combined := strings.Join(lines, "\n")
			if strings.Contains(combined, "No available accounts") || strings.Contains(combined, "no available accounts") {
				return "", errors.New(errs.MsgClaudeCodeAcpBusy)
			}
			return combined, nil
		}

		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = s.cliSvc.CancelSession(sessionID)
			return "", ctx.Err()
		case <-timer.C:
		}
	}
}

// ── 辅助函数 ────────────────────────────────────────────────────────────────

// buildSkillPrompt 构建 Claude Code 技能调用格式的消息。
// 使用 "/技能名 参数" 格式，这是 claude -p 模式下唯一能正确触发
// skill 加载（展开 SKILL.md 完整内容到上下文）的方式。
// XML 标签格式（<command-name>）在非交互模式下不会触发 skill 展开。
func buildSkillPrompt(req GeneratePromptRequest, siblingPrompts []siblingPrompt, projectProfile *promptProjectProfile) string {
	const skillName = "评审项目提示词生成"
	var sb strings.Builder
	sb.WriteString("/")
	sb.WriteString(skillName)
	sb.WriteString(" [PINRU]\ntaskType: ")
	sb.WriteString(internalprompt.NormalizeTaskType(req.TaskType))
	sb.WriteString("\n")

	if len(req.Constraints) > 0 {
		sb.WriteString("constraints: ")
		sb.WriteString(strings.Join(req.Constraints, ","))
		sb.WriteString("\n")
	} else {
		sb.WriteString("constraints: 无约束\n")
	}

	if len(req.Scopes) > 0 {
		sb.WriteString("scope: ")
		sb.WriteString(strings.Join(req.Scopes, ","))
		sb.WriteString("\n")
	}

	if req.AdditionalNotes != nil && strings.TrimSpace(*req.AdditionalNotes) != "" {
		sb.WriteString("notes: ")
		sb.WriteString(strings.TrimSpace(*req.AdditionalNotes))
		sb.WriteString("\n")
	}

	appendProjectRequirementGenerationRules(&sb, req)
	appendTaskSpecificGenerationRules(&sb, req)
	appendProjectProfilePrompt(&sb, projectProfile)

	sb.WriteString("\n输出要求：请返回结构化 JSON，不要包裹 Markdown 代码块。字段必须包含 version、prompt、promptDifficulty。")
	sb.WriteString("promptDifficulty 只能是 简单、一般、困难、地狱 四选一。")
	sb.WriteString("难度请基于最终 prompt 的真实实现成本判断：单文件为简单；跨模块少量文件为一般；跨模块/跨系统多文件、多约束、需要联动验证为困难；只有高耦合、高不确定、验证成本极高时才用地狱。\n")

	if len(siblingPrompts) > 0 {
		sb.WriteString("\n---\n")
		sb.WriteString("同题源已有提示词（来自同一代码仓库的其他试题）：\n")
		sb.WriteString(`要求：新生成的提示词必须在"考察点、切入角度、改动范围、描述措辞"上都与下列已有提示词明显不同，不得出现题目雷同或换皮重复；如果下列提示词已覆盖了该仓库最典型的考察方向，请改从次要的切入点切入。`)
		sb.WriteString("\n\n")
		for i, sp := range siblingPrompts {
			fmt.Fprintf(&sb, "【已有提示词 %d】taskId=%s taskType=%s\n", i+1, sp.TaskID, sp.TaskType)
			sb.WriteString(sp.PromptText)
			sb.WriteString("\n\n")
		}
	}

	return sb.String()
}

func appendProjectRequirementGenerationRules(sb *strings.Builder, req GeneratePromptRequest) {
	normalizedTaskType := internalprompt.NormalizeTaskType(req.TaskType)
	sb.WriteString("\n通用出题规则：必须先基于当前仓库的真实项目结构、已有功能、主要用户路径、状态流和工程约束生成提示词，不要写泛泛产品想法。提示词要像真实项目排期里的研发任务，包含业务背景、触发场景、用户可感知行为、影响范围和交付边界，但不要出现代码片段、文件路径、类名、方法名、接口名、变量名或具体实现步骤；不要在末尾固定追加“验收时...”“验证时...”这类模板句。\n")
	sb.WriteString("任务类型边界：0-1代码生成必须是此前不存在的完整模块、新子系统或新主流程；Feature迭代必须是在已有功能基础上的规则增强、流程延展或能力补齐；Bug修复必须是当前项目中真实存在或由代码迹象支撑的缺陷，写清触发条件、异常表现和业务后果；代码理解聚焦梳理链路和风险；代码重构强调业务结果不变；工程化聚焦构建、依赖、发布或协作稳定性；代码测试围绕高风险流程、边界和回归风险补验证。\n")
	sb.WriteString("质量自检：最终提示词不能只写“优化体验”“完善逻辑”“增强稳定性”这类空话；如果任务涉及导出、统计、预约、状态流转、权限、缓存、异步或跨页面流程，要优先体现数据一致性、异常恢复、边界值、前后端契约或验证链路。难度按理解成本、决策成本和约束复杂度判断，不按文件数量机械判断。\n")

	switch normalizedTaskType {
	case internalprompt.TaskTypeCodeGen:
		sb.WriteString("0-1代码生成额外规则：不要把普通新增按钮、局部配置或小范围能力写成 0-1；必须体现目标用户、关键闭环、状态变化和与现有系统的衔接。\n")
	case internalprompt.TaskTypeFeature:
		sb.WriteString("Feature迭代额外规则：必须说明现有流程哪里不够、扩展后解决什么摩擦，并体现兼容旧行为、上下游影响或多角色协作。\n")
	case internalprompt.TaskTypeTesting:
		sb.WriteString("代码测试额外规则：不要只写补覆盖率，要明确测试对象、正常和异常路径、边界输入、异步时序或回归风险。\n")
	}
}

func appendTaskSpecificGenerationRules(sb *strings.Builder, req GeneratePromptRequest) {
	if internalprompt.NormalizeTaskType(req.TaskType) != internalprompt.TaskTypeBugFix {
		return
	}

	switch promptScopeDifficultyLevel(req.Scopes) {
	case 0, 1:
		sb.WriteString("\nBug修复出题规则：当前范围允许单文件或局部修复，但仍必须基于代码中真实存在、用户可观察的缺陷，不能写成新增功能或泛泛优化。\n")
	case 2:
		sb.WriteString("\nBug修复出题规则：当前范围是模块内多文件，必须选择需要同一业务模块内多个协作部分一起修复的真实缺陷，例如页面交互、状态计算、数据读写、校验或展示链路不一致。不要生成只改一个判断、一个字段、一个按钮状态或一处文案就能解决的简单 bug。\n")
	default:
		sb.WriteString("\nBug修复出题规则：当前范围是跨模块或跨系统多文件，必须选择需要多文件、多层链路联动修复的真实缺陷。优先围绕前后端契约、列表与详情、统计与导出、权限与操作入口、状态流转与库存/订单/通知、缓存与数据刷新等会造成跨模块不一致的场景出题。题目要描述用户看到的异常和修复后的业务结果，但不要点名文件、类、方法或字段。不要生成只改一个判断、一个字段、一个按钮状态或一处文案就能解决的简单 bug。\n")
	}
}

func appendProjectProfilePrompt(sb *strings.Builder, projectProfile *promptProjectProfile) {
	if projectProfile == nil || strings.TrimSpace(projectProfile.ProfileText) == "" {
		sb.WriteString("\n项目分析方式：当前没有可用项目画像缓存，请先快速阅读项目结构和关键文件，再生成提示词。只在必要时深入读取源码，不要无目的全量扫描。\n")
		return
	}

	sb.WriteString("\n---\n")
	sb.WriteString("项目画像缓存（优先使用）：\n")
	sb.WriteString(strings.TrimSpace(projectProfile.ProfileText))
	sb.WriteString("\n\n")
	sb.WriteString("项目分析方式：请优先基于上面的项目画像、任务类型和已有提示词生成任务。只有画像信息不足以支撑真实业务判断时，才补充读取少量相关源码；不要每次从零开始全量扫描项目。\n")
}

func buildCustomProjectPromptDocumentPrompt(projectName string, projectProfile *promptProjectProfile) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "项目名称：%s\n", strings.TrimSpace(projectName))
	sb.WriteString("角色要求：请以有实际研发排期经验的产品经理视角生成提示词，同时理解基本工程实现约束。输出要像真实业务交付任务，但复杂度控制在小中型研发需求，不要写成概念 PRD、营销文案、课堂作业或重型架构改造清单。\n")
	sb.WriteString("请基于当前项目一次性生成提示词需求文档，不要逐条调用单题出题逻辑。\n")
	sb.WriteString("数量要求：只生成 17 条，其中 0-1代码生成 8 条，Feature迭代 8 条，代码理解 1 条；不要生成 Bug修复、代码重构、工程化、代码测试或其他分类。\n")
	sb.WriteString("难度要求：每条必须带任务难度标签，全部统一为【一般】。代码理解固定为【一般】；其余 16 条全部统一为【一般】，不要生成【简单】、【困难】或【地狱】。任务难度整体放开，聚焦真实自然的业务研发需求，不要为了刻意增加难度而强行增加复杂链路压力。\n")
	sb.WriteString("去重要求：17 条提示词之间不得重复或换皮，不得只替换对象名、页面名、状态名后复用同一类需求。每条必须在业务目标、用户路径、状态链路、数据对象、交付边界中至少有两个维度明显不同。输出前必须自检整批内容，如发现重复或相似度过高，必须删除并补充新的不同角度。\n")
	sb.WriteString("内容要求：提示词必须像真实项目排期里的研发任务，包含背景、触发场景和用户可感知行为即可，交付边界点到为止；不要强行拔高复杂度，不要默认堆砌复杂权限、事务一致性、异步恢复、多角色协作、复杂统计口径或跨系统联动。允许灵活、自然地表达业务需求，重点关注项目实际功能的补充和完善。不要出现代码片段、文件路径、类名、方法名、接口名、变量名、命令或具体实现步骤；只有代码理解题允许明确要求生成 README 文档。\n")
	sb.WriteString("文风要求：参考 PINRU 历史提示词的自然写法，每条像真实领题描述的一段中文。不要固定写成“小标题：正文”，不要每条都用冒号切分，也不要先起一个功能名再解释；可以自然使用“当前...”“现在...”“希望...”“新增...”“需要...”等开头，但整批不要同一种句式。不要在每条末尾固定追加“验收时...”“验证时...”“需要确保...”这类验收句；如果必须表达交付结果，要自然融入业务描述里。\n")
	sb.WriteString("0-1代码生成要求：应是此前不存在的中小模块、新页面组或新主流程，不要写成完整大型子系统；重点体现目标用户、核心操作和基本结果。\n")
	sb.WriteString("Feature迭代要求：应是在已有功能基础上补能力或改进流程，说明现有流程哪里不够、扩展后解决什么摩擦即可；尽量控制在适度范围，体现清晰的输入和处理结果衔接即可。\n")
	sb.WriteString("代码理解要求：只输出 1 条且难度固定为【一般】，聚焦梳理一个核心页面、一次提交流程或一条主要数据链路，明确最终需要回答的问题，并要求把梳理结果沉淀为 README 文档；不要要求全量风险盘点，也不要写成需要改代码或改多文件的任务。\n")
	sb.WriteString("输出格式：只返回 Markdown 正文，不要包裹代码块，不要解释生成过程。分类标题固定为 **0-1代码生成**、**Feature迭代**、**代码理解**。每条格式只保留序号、难度标签和自然正文，建议每条 50-100 字。例如：1. 【一般】当前会员预约后到场情况不清楚，管理员无法知道实际到课率。需要补一个签到核销入口，把预约状态和到场结果记录下来，并在取消、迟到和重复核销时给出清楚反馈，方便后续查看课程运营情况。不要输出成“会员签到核销子系统：...”这类固定标题格式。\n")

	if projectProfile == nil || strings.TrimSpace(projectProfile.ProfileText) == "" {
		sb.WriteString("\n项目分析方式：当前没有可用项目画像缓存，请先快速阅读项目结构和关键文件，再生成文档；只在必要时读取源码，不要无目的全量扫描。\n")
		return sb.String()
	}

	sb.WriteString("\n---\n")
	sb.WriteString("项目画像缓存（优先使用）：\n")
	sb.WriteString(strings.TrimSpace(projectProfile.ProfileText))
	sb.WriteString("\n\n项目分析方式：优先基于上面的项目画像生成文档。只有画像信息不足以支撑真实业务判断时，才补充读取少量相关源码。\n")
	return sb.String()
}

func cleanCustomProjectPromptDocument(output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return ""
	}
	trimmed = stripMarkdownFence(trimmed)
	if idx := strings.Index(trimmed, "0-1代码生成"); idx >= 0 {
		start := idx
		prefix := trimmed[:idx]
		for _, marker := range []string{"**", "### ", "## ", "# "} {
			if strings.HasSuffix(prefix, marker) {
				start = len(prefix) - len(marker)
				break
			}
		}
		trimmed = trimmed[start:]
	}
	return strings.TrimSpace(trimmed)
}

func stripMarkdownFence(text string) string {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) >= 2 && strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
		lines = lines[1:]
		if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
			lines = lines[:len(lines)-1]
		}
		return strings.TrimSpace(strings.Join(lines, "\n"))
	}
	return trimmed
}

type siblingPrompt struct {
	TaskID     string
	TaskType   string
	PromptText string
}

type duplicatePromptMatch struct {
	TaskID     string
	TaskType   string
	PromptText string
	Reason     string
	Confidence float64
}

type semanticDuplicateDecision struct {
	IsDuplicate bool    `json:"isDuplicate"`
	Confidence  float64 `json:"confidence"`
	TaskID      string  `json:"taskId"`
	Reason      string  `json:"reason"`
}

const (
	semanticDuplicateFullCandidateLimit    = 6
	semanticDuplicateExcerptCandidateLimit = 12
	semanticDuplicateExcerptRuneLimit      = 120
	semanticDuplicateLocalJudgeThreshold   = 0.015
	semanticDuplicateConfidenceThreshold   = 0.65
	semanticDuplicateJudgeTimeout          = 90 * time.Second
	generationContextPromptLimit           = 8
	generationContextExcerptRuneLimit      = 120
)

func collectPromptDedupSources(task *store.Task, siblings []siblingPrompt) []siblingPrompt {
	result := make([]siblingPrompt, 0, len(siblings)+1)
	if task != nil && task.PromptText != nil {
		if text := strings.TrimSpace(*task.PromptText); text != "" {
			result = append(result, siblingPrompt{
				TaskID:     task.ID,
				TaskType:   task.TaskType,
				PromptText: text,
			})
		}
	}
	result = append(result, siblings...)
	return result
}

func collectSiblingPrompts(tasks []store.Task) []siblingPrompt {
	result := make([]siblingPrompt, 0, len(tasks))
	for _, t := range tasks {
		if t.PromptText == nil {
			continue
		}
		text := strings.TrimSpace(*t.PromptText)
		if text == "" {
			continue
		}
		result = append(result, siblingPrompt{
			TaskID:     t.ID,
			TaskType:   t.TaskType,
			PromptText: text,
		})
	}
	return result
}

func promptGenerationContext(existingPrompts []siblingPrompt) []siblingPrompt {
	if len(existingPrompts) == 0 {
		return nil
	}
	limit := generationContextPromptLimit
	if len(existingPrompts) < limit {
		limit = len(existingPrompts)
	}
	result := make([]siblingPrompt, 0, limit)
	for _, prompt := range existingPrompts[:limit] {
		result = append(result, siblingPrompt{
			TaskID:     prompt.TaskID,
			TaskType:   prompt.TaskType,
			PromptText: truncateRunes(prompt.PromptText, generationContextExcerptRuneLimit),
		})
	}
	return result
}

func normalizePromptForDuplicateCheck(prompt string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(prompt)), " ")
}

func isDuplicatePrompt(promptText string, existingPrompts []siblingPrompt) bool {
	_, ok := findExactDuplicatePromptMatch(promptText, existingPrompts)
	return ok
}

func findExactDuplicatePromptMatch(promptText string, existingPrompts []siblingPrompt) (duplicatePromptMatch, bool) {
	normalized := normalizePromptForDuplicateCheck(promptText)
	if normalized == "" {
		return duplicatePromptMatch{}, false
	}
	for _, existing := range existingPrompts {
		if normalized == normalizePromptForDuplicateCheck(existing.PromptText) {
			return duplicatePromptMatch{
				TaskID:     existing.TaskID,
				TaskType:   existing.TaskType,
				PromptText: existing.PromptText,
				Reason:     "文本完全重复",
				Confidence: 1,
			}, true
		}
	}
	return duplicatePromptMatch{}, false
}

func (s *PromptService) findDuplicatePromptMatch(ctx context.Context, workDir, promptText string, existingPrompts []siblingPrompt, model string) (duplicatePromptMatch, bool) {
	if match, ok := findExactDuplicatePromptMatch(promptText, existingPrompts); ok {
		return match, true
	}
	candidates := semanticDuplicateCandidates(promptText, existingPrompts, semanticDuplicateExcerptCandidateLimit)
	if len(candidates) == 0 {
		return duplicatePromptMatch{}, false
	}
	if candidates[0].Confidence < semanticDuplicateLocalJudgeThreshold {
		slog.Info("semantic duplicate judge skipped for low local similarity",
			"candidate_count", len(candidates),
			"top_score", candidates[0].Confidence,
			"threshold", semanticDuplicateLocalJudgeThreshold,
		)
		return duplicatePromptMatch{}, false
	}
	decision, err := s.judgeSemanticDuplicatePrompt(ctx, workDir, promptText, candidates, model)
	if err != nil {
		slog.Warn("semantic duplicate judge failed",
			"candidate_count", len(candidates),
			"error", err,
		)
		return duplicatePromptMatch{}, false
	}
	if !decision.IsDuplicate || decision.Confidence < semanticDuplicateConfidenceThreshold {
		return duplicatePromptMatch{}, false
	}
	match := candidates[0]
	if strings.TrimSpace(decision.TaskID) != "" {
		for _, candidate := range candidates {
			if strings.TrimSpace(candidate.TaskID) == strings.TrimSpace(decision.TaskID) {
				match = candidate
				break
			}
		}
	}
	match.Reason = strings.TrimSpace(decision.Reason)
	if match.Reason == "" {
		match.Reason = "模型判定语义重复"
	}
	match.Confidence = decision.Confidence
	return match, true
}

func semanticDuplicateCandidates(promptText string, existingPrompts []siblingPrompt, limit int) []duplicatePromptMatch {
	if limit <= 0 {
		limit = len(existingPrompts)
	}
	newTerms := extractPromptSimilarityTerms(promptText)
	candidates := make([]duplicatePromptMatch, 0, len(existingPrompts))

	for _, existing := range existingPrompts {
		oldTerms := extractPromptSimilarityTerms(existing.PromptText)
		score := 0.0
		if len(newTerms) > 0 && len(oldTerms) > 0 {
			score = jaccardTermSimilarity(newTerms, oldTerms)
		}
		candidates = append(candidates, duplicatePromptMatch{
			TaskID:     existing.TaskID,
			TaskType:   existing.TaskType,
			PromptText: existing.PromptText,
			Reason:     fmt.Sprintf("本地语义候选相似度 %.2f", score),
			Confidence: score,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Confidence > candidates[j].Confidence
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates
}

func extractPromptSimilarityTerms(prompt string) map[string]struct{} {
	text := normalizePromptSimilarityText(prompt)
	terms := make(map[string]struct{})
	for _, field := range strings.Fields(text) {
		addPromptTerm(terms, field)
	}

	runes := []rune(strings.ReplaceAll(text, " ", ""))
	for n := 2; n <= 4; n++ {
		if len(runes) < n {
			continue
		}
		for i := 0; i <= len(runes)-n; i++ {
			terms[string(runes[i:i+n])] = struct{}{}
		}
	}
	return terms
}

func normalizePromptSimilarityText(prompt string) string {
	text := strings.ToLower(strings.TrimSpace(prompt))
	replacer := strings.NewReplacer(
		"，", " ", "。", " ", "；", " ", "：", " ", "、", " ", "（", " ", "）", " ",
		"“", " ", "”", " ", "\"", " ", "'", " ", "\n", " ", "\t", " ", "！", " ",
		"？", " ", ":", " ", ",", " ", ".", " ", ";", " ", "!", " ", "?", " ",
		"(", " ", ")", " ", "[", " ", "]", " ", "{", " ", "}", " ",
	)
	return strings.Join(strings.Fields(replacer.Replace(text)), " ")
}

func addPromptTerm(terms map[string]struct{}, field string) {
	field = strings.TrimSpace(field)
	if len([]rune(field)) < 2 {
		return
	}
	terms[field] = struct{}{}
}

func countTermOverlap(left, right map[string]struct{}) int {
	count := 0
	for term := range left {
		if _, ok := right[term]; ok {
			count++
		}
	}
	return count
}

func jaccardTermSimilarity(left, right map[string]struct{}) float64 {
	intersection := countTermOverlap(left, right)
	if intersection == 0 {
		return 0
	}
	union := len(left) + len(right) - intersection
	if union <= 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func (s *PromptService) judgeSemanticDuplicatePrompt(ctx context.Context, workDir, promptText string, candidates []duplicatePromptMatch, model string) (semanticDuplicateDecision, error) {
	judgePrompt := buildSemanticDuplicateJudgePrompt(promptText, candidates)
	var output string
	var err error
	if s.duplicateJudge != nil {
		return s.duplicateJudge(ctx, workDir, judgePrompt, model)
	}
	judgeCtx, cancel := context.WithTimeout(ctx, semanticDuplicateJudgeTimeout)
	defer cancel()
	output, err = s.executeCliRaw(judgeCtx, workDir, judgePrompt, model)
	if err != nil {
		return semanticDuplicateDecision{}, err
	}
	return extractSemanticDuplicateDecision(output)
}

func buildSemanticDuplicateJudgePrompt(promptText string, candidates []duplicatePromptMatch) string {
	var sb strings.Builder
	sb.WriteString("你是提示词语义去重审核器。请判断新提示词和历史提示词是否属于同一业务能力或同一考察点的换皮重复。\n")
	sb.WriteString("判重标准：只要核心业务对象、目标能力、主要数据链路和验收重点高度一致，就算重复；即使措辞不同、补充了少量细节也算重复。若只是同一系统里的不同功能，不算重复。\n")
	sb.WriteString("历史候选已按本地通用相似度排序。完整候选用于精判，摘要候选用于兜底识别明显重复。\n")
	sb.WriteString("请从给定历史提示词中找最相似的一条进行判断。只输出 JSON，不要 Markdown。格式：{\"isDuplicate\":true|false,\"confidence\":0到1,\"taskId\":\"命中的历史 taskId，非重复时为空\",\"reason\":\"一句中文原因\"}\n\n")
	fullCandidates, excerptCandidates := splitSemanticDuplicateJudgeCandidates(candidates)
	sb.WriteString("完整历史候选：\n")
	for i, candidate := range fullCandidates {
		fmt.Fprintf(&sb, "【完整 %d】taskId=%s taskType=%s score=%.2f\n", i+1, candidate.TaskID, candidate.TaskType, candidate.Confidence)
		sb.WriteString(strings.TrimSpace(candidate.PromptText))
		sb.WriteString("\n\n")
	}
	if len(excerptCandidates) > 0 {
		sb.WriteString("摘要历史候选：\n")
		for i, candidate := range excerptCandidates {
			fmt.Fprintf(&sb, "【摘要 %d】taskId=%s taskType=%s score=%.2f\n", i+1, candidate.TaskID, candidate.TaskType, candidate.Confidence)
			sb.WriteString(truncateRunes(strings.TrimSpace(candidate.PromptText), semanticDuplicateExcerptRuneLimit))
			sb.WriteString("\n\n")
		}
	}
	sb.WriteString("\n\n新提示词：\n")
	sb.WriteString(strings.TrimSpace(promptText))
	sb.WriteString("\n")
	return sb.String()
}

func splitSemanticDuplicateJudgeCandidates(candidates []duplicatePromptMatch) ([]duplicatePromptMatch, []duplicatePromptMatch) {
	if len(candidates) <= semanticDuplicateFullCandidateLimit {
		return candidates, nil
	}
	full := candidates[:semanticDuplicateFullCandidateLimit]
	excerpt := candidates[semanticDuplicateFullCandidateLimit:]
	return full, excerpt
}

func truncateRunes(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}

func extractSemanticDuplicateDecision(output string) (semanticDuplicateDecision, error) {
	for _, candidate := range extractAllJSONObjects(output) {
		var decision semanticDuplicateDecision
		if err := json.Unmarshal([]byte(candidate), &decision); err == nil {
			return decision, nil
		}
	}
	trimmed := strings.TrimSpace(TrimPromptCodeFence(output))
	var decision semanticDuplicateDecision
	if err := json.Unmarshal([]byte(trimmed), &decision); err != nil {
		return semanticDuplicateDecision{}, err
	}
	return decision, nil
}

func buildDuplicateRegenerationPrompt(req GeneratePromptRequest, existingPrompts []siblingPrompt, duplicatePrompt string, match duplicatePromptMatch, projectProfile *promptProjectProfile) string {
	base := buildSkillPrompt(req, existingPrompts, projectProfile)
	var sb strings.Builder
	sb.WriteString(base)
	sb.WriteString("\n---\n")
	sb.WriteString("自动重生成要求：上一轮生成结果与已有提示词重复或语义高度相似，必须重新生成一条不同的提示词。\n")
	sb.WriteString("新提示词不得只替换少量同义词，必须更换业务切入点、验收重点和改动边界。\n")
	if strings.TrimSpace(match.TaskID) != "" {
		fmt.Fprintf(&sb, "重复命中的历史提示词：taskId=%s taskType=%s\n", match.TaskID, match.TaskType)
	}
	if strings.TrimSpace(match.Reason) != "" {
		sb.WriteString("重复原因：")
		sb.WriteString(strings.TrimSpace(match.Reason))
		sb.WriteString("\n")
	}
	sb.WriteString("上一轮重复结果：\n")
	sb.WriteString(strings.TrimSpace(duplicatePrompt))
	sb.WriteString("\n")
	return sb.String()
}

func estimatePromptDifficulty(req GeneratePromptRequest, promptText string) string {
	scopeLevel := promptScopeDifficultyLevel(req.Scopes)
	if scopeLevel <= 1 {
		if scopeLevel == 1 {
			return "简单"
		}
		return store.DefaultPromptDifficulty
	}
	if scopeLevel == 2 {
		return store.DefaultPromptDifficulty
	}

	meaningfulConstraints := countMeaningfulPromptConstraints(req.Constraints)
	hasNotes := req.AdditionalNotes != nil && strings.TrimSpace(*req.AdditionalNotes) != ""
	length := len([]rune(strings.TrimSpace(promptText)))

	if scopeLevel == 3 {
		if meaningfulConstraints <= 1 && !hasNotes && length <= 180 {
			return store.DefaultPromptDifficulty
		}
		return "困难"
	}

	if meaningfulConstraints >= 3 || (hasNotes && length > 180) || length > 300 {
		return "地狱"
	}
	return "困难"
}

func promptScopeDifficultyLevel(scopes []string) int {
	level := 0
	for _, scope := range scopes {
		switch strings.TrimSpace(scope) {
		case "单文件":
			if level < 1 {
				level = 1
			}
		case "模块内多文件":
			if level < 2 {
				level = 2
			}
		case "跨模块多文件":
			if level < 3 {
				level = 3
			}
		case "跨系统多模块":
			if level < 4 {
				level = 4
			}
		}
	}
	return level
}

func countMeaningfulPromptConstraints(constraints []string) int {
	count := 0
	for _, constraint := range constraints {
		trimmed := strings.TrimSpace(constraint)
		if trimmed != "" && trimmed != "无约束" {
			count++
		}
	}
	return count
}

func NormalizePromptDifficultyLabel(value string) string {
	switch strings.TrimSpace(value) {
	case "简单":
		return "简单"
	case "一般":
		return store.DefaultPromptDifficulty
	case "困难":
		return "困难"
	case "地狱":
		return "地狱"
	default:
		return ""
	}
}

type promptProviderSelection struct {
	Name  string
	Model string
}

func buildPolishSkillPrompt(text string) string {
	trimmed := strings.TrimSpace(text)
	return strings.Join([]string{
		"/humanizer-zh",
		"",
		"请把下面内容改成更自然、更口语化的业务描述。",
		"不要出现代码片段、伪代码、命令、路径、变量名或技术实现细节。",
		"重点保留业务现象、用户感知、场景变化和需要补齐的业务处理。",
		"只返回润色后的正文。",
		"",
		trimmed,
	}, "\n")
}

func defaultPolishWorkDir() string {
	workDir, err := os.Getwd()
	if err != nil || strings.TrimSpace(workDir) == "" {
		return "."
	}
	return workDir
}

// resolveProviderForPromptGeneration 从 LLM provider 配置中解析出提示词生成使用的 Claude Code provider。
func resolveProviderForPromptGeneration(st *store.Store, requestedID *string) (promptProviderSelection, error) {
	providers, err := st.ListLLMProviders()
	if err != nil {
		return promptProviderSelection{}, err
	}
	if len(providers) == 0 {
		return promptProviderSelection{
			Name:  "Claude Code CLI",
			Model: defaultPromptGenerationModel,
		}, nil
	}

	if requestedID != nil && strings.TrimSpace(*requestedID) != "" {
		selected := selectProvider(providers, requestedID)
		if selected == nil {
			return promptProviderSelection{}, errors.New(errs.MsgClaudeCodeAcpMissing)
		}
		if selected.ProviderType != "claude_code_acp" {
			return promptProviderSelection{}, errors.New(errs.MsgClaudeCodeAcpOnly)
		}
		return buildPromptProviderSelection(*selected), nil
	}

	if selected := selectDefaultClaudeCodeProvider(providers); selected != nil {
		return buildPromptProviderSelection(*selected), nil
	}
	if selected := selectFirstClaudeCodeProvider(providers); selected != nil {
		return buildPromptProviderSelection(*selected), nil
	}

	return promptProviderSelection{}, errors.New(errs.MsgClaudeCodeAcpNotConfigured)
}

func buildPromptProviderSelection(provider store.LLMProvider) promptProviderSelection {
	name := strings.TrimSpace(provider.Name)
	if name == "" {
		name = "Claude Code CLI"
	}
	model := strings.TrimSpace(provider.Model)
	if model == "" {
		model = defaultPromptGenerationModel
	}
	return promptProviderSelection{
		Name:  name,
		Model: model,
	}
}

func resolveProviderForPolish(st *store.Store, requestedID *string) (promptProviderSelection, error) {
	return resolveProviderForPromptGeneration(st, requestedID)
}

func selectDefaultClaudeCodeProvider(providers []store.LLMProvider) *store.LLMProvider {
	for i := range providers {
		if providers[i].IsDefault && providers[i].ProviderType == "claude_code_acp" {
			return &providers[i]
		}
	}
	return nil
}

func selectFirstClaudeCodeProvider(providers []store.LLMProvider) *store.LLMProvider {
	for i := range providers {
		if providers[i].ProviderType == "claude_code_acp" {
			return &providers[i]
		}
	}
	return nil
}

func normalizePromptGenerationError(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "提示词生成超时，请稍后重试"
	case errors.Is(err, context.Canceled):
		return "提示词生成已取消"
	default:
		msg := err.Error()
		if strings.Contains(msg, "No available accounts") || strings.Contains(msg, "no available accounts") {
			return "Claude Code ACP 账号池暂时耗尽（503），请稍后重试或检查 ACP 配置"
		}
		return msg
	}
}

func selectProvider(providers []store.LLMProvider, requestedID *string) *store.LLMProvider {
	if requestedID != nil && strings.TrimSpace(*requestedID) != "" {
		for i := range providers {
			if providers[i].ID == *requestedID {
				return &providers[i]
			}
		}
		return nil
	}
	for i := range providers {
		if providers[i].IsDefault {
			return &providers[i]
		}
	}
	if len(providers) > 0 {
		return &providers[0]
	}
	return nil
}

// cliAdditionalDirs 返回 CLI Agent 需要访问的额外目录（执行手册目录）。
func cliAdditionalDirs() []string {
	dir := internalprompt.DefaultManualDir()
	if dir == "" {
		return nil
	}
	return []string{dir}
}

func isCustomQuestionBankItem(item store.QuestionBankItem) bool {
	return strings.EqualFold(strings.TrimSpace(item.SourceKind), "local_directory") &&
		strings.HasPrefix(strings.ToLower(strings.TrimSpace(item.OriginRef)), "custom:")
}

func normalizeCustomProjectDocumentNames(projectNames []string) []string {
	seen := make(map[string]struct{}, len(projectNames))
	result := make([]string, 0, len(projectNames))
	for _, name := range projectNames {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func buildCustomProjectPromptDocumentPath(rootPath, projectName string, now time.Time) string {
	fileName := fmt.Sprintf("%s_提示词_%s.md", sanitizePromptDocumentFileName(projectName), now.Format("0102"))
	return util.NormalizePath(filepath.Join(rootPath, fileName))
}

func inferCustomProjectNameFromPromptDocumentPath(path string) string {
	base := strings.TrimSuffix(filepath.Base(strings.TrimSpace(path)), filepath.Ext(path))
	if idx := strings.LastIndex(base, "_提示词_"); idx > 0 {
		return strings.TrimSpace(base[:idx])
	}
	return strings.TrimSpace(base)
}

func sanitizePromptDocumentFileName(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "custom_project"
	}
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "-",
		"?", "-",
		"\"", "",
		"<", "-",
		">", "-",
		"|", "-",
	)
	cleaned := strings.TrimSpace(replacer.Replace(trimmed))
	cleaned = strings.Join(strings.Fields(cleaned), "")
	if cleaned == "" || cleaned == "." || cleaned == ".." {
		return "custom_project"
	}
	return cleaned
}

func (s *PromptService) resolveProviderForTest(provider store.LLMProvider) (store.LLMProvider, error) {
	provider.ID = strings.TrimSpace(provider.ID)
	provider.Name = strings.TrimSpace(provider.Name)
	provider.ProviderType = strings.TrimSpace(provider.ProviderType)
	provider.Model = strings.TrimSpace(provider.Model)

	if provider.ID != "" && !llm.IsACPProvider(provider.ProviderType) && strings.TrimSpace(provider.APIKey) == "" {
		storedProvider, err := s.store.GetLLMProvider(provider.ID)
		if err != nil {
			return store.LLMProvider{}, err
		}
		if storedProvider != nil && strings.TrimSpace(storedProvider.APIKey) != "" {
			provider.APIKey = storedProvider.APIKey
		}
	}

	if provider.ProviderType == "" {
		return store.LLMProvider{}, errors.New(errs.MsgProviderTypeRequired)
	}
	if provider.Model == "" {
		return store.LLMProvider{}, errors.New(errs.MsgModelNameRequired)
	}

	baseURL := provider.BaseURL
	if baseURL != nil {
		trimmed := strings.TrimSpace(*baseURL)
		if trimmed == "" {
			baseURL = nil
		} else {
			baseURL = &trimmed
		}
	}
	provider.BaseURL = baseURL

	return provider, nil
}

// ── 润色文本 ────────────────────────────────────────────────────────────────

// PolishTextRequest 润色请求。
type PolishTextRequest struct {
	Text       string  `json:"text"`
	ProviderID *string `json:"providerId"`
}

// PolishTextResult 润色结果。
type PolishTextResult struct {
	PolishedText string `json:"polishedText"`
	ProviderName string `json:"providerName"`
	Model        string `json:"model"`
}

// PolishText 使用 Claude Code CLI 执行 /humanizer-zh 并返回输出正文。
func (s *PromptService) PolishText(req PolishTextRequest) (*PolishTextResult, error) {
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return nil, errors.New(errs.MsgPolishTextRequired)
	}

	if _, err := s.cliSvc.CheckCLI(); err != nil {
		return nil, errors.New(errs.MsgClaudeCodeCliNotInstalledInstallGuide)
	}

	selection, err := resolveProviderForPolish(s.store, req.ProviderID)
	if err != nil {
		return nil, err
	}

	workDir := defaultPolishWorkDir()
	skillPrompt := buildPolishSkillPrompt(text)
	slog.Info("PolishText started", "model", selection.Model, "provider", selection.Name, "textLen", len(text))
	polished, err := s.executeCliHumanizer(context.Background(), workDir, skillPrompt, selection.Model)
	if err != nil {
		return nil, fmt.Errorf(errs.FmtPolishFailed, err)
	}

	polished = strings.TrimSpace(polished)
	if polished == "" {
		return nil, errors.New(errs.MsgHumanizerEmpty)
	}

	slog.Info("PolishText completed", "model", selection.Model, "resultLen", len(polished))
	return &PolishTextResult{
		PolishedText: polished,
		ProviderName: selection.Name,
		Model:        selection.Model,
	}, nil
}
