package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	appgit "github.com/blueship581/pinru/app/git"
	"github.com/blueship581/pinru/app/testutil"
	"github.com/blueship581/pinru/internal/store"
)

func TestParseCustomPromptDocumentEntries(t *testing.T) {
	content := strings.Join([]string{
		"**0-1代码生成**",
		"",
		"1. 【简单】做一个学生入住登记入口，支持老师录入学生和床位关系。",
		"   需要保留异常提示。",
		"",
		"**Feature迭代**",
		"",
		"1. 在现有列表里增加入住状态筛选。",
		"",
		"**代码理解**",
		"",
		"- 【困难】梳理住宿状态从分配到退宿的流转。",
	}, "\n")

	entries := parseCustomPromptDocumentEntries(content)
	if len(entries) != 3 {
		t.Fatalf("entries len = %d, want 3: %+v", len(entries), entries)
	}
	if entries[0].TaskType != "0-1代码生成" || entries[0].PromptDifficulty != "简单" {
		t.Fatalf("entry[0] = %+v", entries[0])
	}
	if !strings.Contains(entries[0].PromptText, "需要保留异常提示") {
		t.Fatalf("entry[0].PromptText missing continuation: %q", entries[0].PromptText)
	}
	if entries[1].TaskType != "Feature迭代" || entries[1].PromptDifficulty != store.DefaultPromptDifficulty {
		t.Fatalf("entry[1] = %+v", entries[1])
	}
	if entries[2].TaskType != "代码理解" || entries[2].PromptDifficulty != "困难" {
		t.Fatalf("entry[2] = %+v", entries[2])
	}
}

func TestCreateTasksFromCustomPromptDocumentsCreatesTasksAndPromptArtifacts(t *testing.T) {
	testStore := testutil.OpenTestStore(t)
	defer testStore.Close()

	cloneBase := t.TempDir()
	project := store.Project{
		ID:                "project-custom-doc-task",
		Name:              "Custom Doc",
		CloneBasePath:     cloneBase,
		Models:            "ORIGIN,model-a",
		SourceModelFolder: "ORIGIN",
	}
	if err := testStore.CreateProject(project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	sourcePath := filepath.Join(t.TempDir(), "zw-001-source")
	if err := os.MkdirAll(filepath.Join(sourcePath, "src"), 0o755); err != nil {
		t.Fatalf("MkdirAll(sourcePath) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "src", "main.ts"), []byte("export const value = 1;\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	if err := testStore.UpsertQuestionBankItem(store.QuestionBankItem{
		ProjectConfigID: project.ID,
		QuestionID:      1001,
		DisplayName:     "zw-001",
		SourceKind:      "local_directory",
		SourcePath:      sourcePath,
		OriginRef:       "custom:zw-001",
		Status:          "ready",
	}); err != nil {
		t.Fatalf("UpsertQuestionBankItem() error = %v", err)
	}

	docPath := filepath.Join(t.TempDir(), "zw-001_提示词_0602.md")
	docContent := strings.Join([]string{
		"**0-1代码生成**",
		"",
		"1. 【简单】新增完整地址簿能力，让用户维护常用地址并在发布流程复用。",
		"2. 【一般】新增发布审核台，支持管理员查看待审核内容并批量处理。",
		"",
		"**Feature迭代**",
		"",
		"1. 【困难】在现有发布流程里补充草稿自动保存和恢复能力。",
		"",
		"**代码理解**",
		"",
		"1. 【一般】梳理发布流程从填写到提交完成的关键状态流。",
	}, "\n")
	if err := os.WriteFile(docPath, []byte(docContent), 0o644); err != nil {
		t.Fatalf("WriteFile(doc) error = %v", err)
	}

	svc := New(testStore, appgit.New(testStore))
	result, err := svc.CreateTasksFromCustomPromptDocuments(CreateTasksFromCustomPromptDocumentsRequest{
		ProjectID:     project.ID,
		DocumentPaths: []string{docPath},
	})
	if err != nil {
		t.Fatalf("CreateTasksFromCustomPromptDocuments() error = %v", err)
	}
	if result.CreatedCount != 4 || result.ErrorCount != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(result.Details) != 1 || result.Details[0].ParsedCount != 4 {
		t.Fatalf("unexpected detail: %+v", result.Details)
	}

	tasks, err := testStore.ListTasks(&project.ID)
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(tasks) != 4 {
		t.Fatalf("tasks len = %d, want 4", len(tasks))
	}

	seenTypes := map[string]int{}
	for _, task := range tasks {
		seenTypes[task.TaskType]++
		if task.Status != "PromptReady" {
			t.Fatalf("task %s status = %q, want PromptReady", task.ID, task.Status)
		}
		if task.PromptText == nil || strings.TrimSpace(*task.PromptText) == "" {
			t.Fatalf("task %s prompt missing", task.ID)
		}
		if task.LocalPath == nil {
			t.Fatalf("task %s local path nil", task.ID)
		}
		artifactPath := filepath.Join(*task.LocalPath, "任务提示词.md")
		artifact, err := os.ReadFile(artifactPath)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", artifactPath, err)
		}
		if strings.TrimSpace(string(artifact)) != strings.TrimSpace(*task.PromptText) {
			t.Fatalf("artifact mismatch for %s", task.ID)
		}
		sourceFolderName := filepath.Base(*task.LocalPath)
		if _, err := os.Stat(filepath.Join(*task.LocalPath, sourceFolderName, "src", "main.ts")); err != nil {
			t.Fatalf("source copy missing for %s: %v", task.ID, err)
		}
		if _, err := os.Stat(filepath.Join(*task.LocalPath, "model-a", "src", "main.ts")); err != nil {
			if !os.IsNotExist(err) {
				t.Fatalf("unexpected model copy stat error for %s: %v", task.ID, err)
			}
		} else {
			t.Fatalf("model copy should not exist for custom prompt task %s", task.ID)
		}
	}
	if seenTypes["0-1代码生成"] != 2 || seenTypes["Feature迭代"] != 1 || seenTypes["代码理解"] != 1 {
		t.Fatalf("seenTypes = %+v", seenTypes)
	}
}

func TestCreateTasksFromCustomPromptDocumentsCopiesExistingNodeModules(t *testing.T) {
	testStore := testutil.OpenTestStore(t)
	defer testStore.Close()

	cloneBase := t.TempDir()
	project := store.Project{
		ID:                "project-custom-doc-node-modules",
		Name:              "Custom Doc Node",
		CloneBasePath:     cloneBase,
		Models:            "ORIGIN",
		SourceModelFolder: "ORIGIN",
	}
	if err := testStore.CreateProject(project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	sourcePath := filepath.Join(t.TempDir(), "zw-node-source")
	if err := os.MkdirAll(filepath.Join(sourcePath, "node_modules", "left-pad"), 0o755); err != nil {
		t.Fatalf("MkdirAll(node_modules) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "package.json"), []byte(`{"scripts":{"dev":"vite"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile(package.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "node_modules", "left-pad", "index.js"), []byte("module.exports = function(){};\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(node module) error = %v", err)
	}
	if err := testStore.UpsertQuestionBankItem(store.QuestionBankItem{
		ProjectConfigID: project.ID,
		QuestionID:      1002,
		DisplayName:     "zw-node",
		SourceKind:      "local_directory",
		SourcePath:      sourcePath,
		OriginRef:       "custom:zw-node",
		Status:          "ready",
	}); err != nil {
		t.Fatalf("UpsertQuestionBankItem() error = %v", err)
	}

	docPath := filepath.Join(t.TempDir(), "zw-node_提示词_0602.md")
	docContent := strings.Join([]string{
		"**0-1代码生成**",
		"",
		"1. 【简单】新增一个前端调试入口，方便快速查看当前页面运行状态。",
	}, "\n")
	if err := os.WriteFile(docPath, []byte(docContent), 0o644); err != nil {
		t.Fatalf("WriteFile(doc) error = %v", err)
	}

	svc := New(testStore, appgit.New(testStore))
	result, err := svc.CreateTasksFromCustomPromptDocuments(CreateTasksFromCustomPromptDocumentsRequest{
		ProjectID:     project.ID,
		DocumentPaths: []string{docPath},
	})
	if err != nil {
		t.Fatalf("CreateTasksFromCustomPromptDocuments() error = %v", err)
	}
	if result.CreatedCount != 1 || result.ErrorCount != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	tasks, err := testStore.ListTasks(&project.ID)
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].LocalPath == nil {
		t.Fatalf("tasks = %+v", tasks)
	}
	sourceFolderName := filepath.Base(*tasks[0].LocalPath)
	if _, err := os.Stat(filepath.Join(*tasks[0].LocalPath, sourceFolderName, "node_modules", "left-pad", "index.js")); err != nil {
		t.Fatalf("node_modules copy missing: %v", err)
	}
}
