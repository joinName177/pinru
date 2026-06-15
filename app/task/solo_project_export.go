package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type SoloProjectXlsxExportResult struct {
	ProjectName       string `json:"projectName"`
	OutputPath        string `json:"outputPath"`
	ValidationPath    string `json:"validationPath"`
	Rows              int    `json:"rows"`
	ValidationRows    int    `json:"validationRows"`
	DuplicateSessions int    `json:"duplicateSessions"`
	EmptyRepoURL      int    `json:"emptyRepoUrl"`
	EmptyCommit       int    `json:"emptyCommit"`
	MissingPRRecords  int    `json:"missingPrRecords"`
	RepoURLFilled     int    `json:"repoUrlFilled"`
}

type soloProjectXlsxExportStats struct {
	Rows              int `json:"rows"`
	ValidationRows    int `json:"validation_rows"`
	DuplicateSessions int `json:"duplicate_sessions"`
	EmptyRepoURL      int `json:"empty_repo_url"`
	EmptyCommit       int `json:"empty_commit"`
	MissingPRRecords  int `json:"missing_pr_records"`
	RepoURLFilled     int `json:"repo_url_filled"`
}

func (s *TaskService) ExportSoloProjectXlsx(projectName string) (*SoloProjectXlsxExportResult, error) {
	var err error
	if strings.TrimSpace(projectName) == "" {
		projectName, err = s.activeProjectName()
		if err != nil {
			return nil, err
		}
	}
	if err != nil {
		return nil, err
	}
	projectName = strings.TrimSpace(projectName)
	if projectName == "" {
		return nil, errors.New("项目名称不能为空")
	}

	cwd, err := findRepoRoot()
	if err != nil {
		return nil, err
	}
	scriptPath := filepath.Join(cwd, "scripts", "export_solo_project_xlsx.py")
	templatePath := filepath.Join(cwd, "public", "bmymoban.xlsx")
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, fmt.Errorf("找不到导出脚本：%s", scriptPath)
	}
	if _, err := os.Stat(templatePath); err != nil {
		return nil, fmt.Errorf("找不到导出模板：%s", templatePath)
	}

	outputDir := filepath.Join(cwd, "exports")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, err
	}
	dateLabel := time.Now().Format("2006-01-02")
	outputPath := filepath.Join(outputDir, fmt.Sprintf("%s-session-review-export-%s-repoid.xlsx", projectName, dateLabel))
	reportPath := filepath.Join(outputDir, fmt.Sprintf("%s-session-review-export-%s-repoid-validation.md", projectName, dateLabel))

	cmd := exec.Command(
		"python3",
		scriptPath,
		"--project", projectName,
		"--db", s.store.DBPath(),
		"--template", templatePath,
		"--output", outputPath,
		"--report-output", reportPath,
	)
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("导出失败：%v\n%s", err, strings.TrimSpace(string(out)))
	}

	stats := parseSoloProjectExportStats(out)
	return &SoloProjectXlsxExportResult{
		ProjectName:       projectName,
		OutputPath:        outputPath,
		ValidationPath:    reportPath,
		Rows:              stats.Rows,
		ValidationRows:    stats.ValidationRows,
		DuplicateSessions: stats.DuplicateSessions,
		EmptyRepoURL:      stats.EmptyRepoURL,
		EmptyCommit:       stats.EmptyCommit,
		MissingPRRecords:  stats.MissingPRRecords,
		RepoURLFilled:     stats.RepoURLFilled,
	}, nil
}

func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		scriptPath := filepath.Join(dir, "scripts", "export_solo_project_xlsx.py")
		templatePath := filepath.Join(dir, "public", "bmymoban.xlsx")
		if _, scriptErr := os.Stat(scriptPath); scriptErr == nil {
			if _, templateErr := os.Stat(templatePath); templateErr == nil {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("找不到仓库根目录：%s", wd)
}

func (s *TaskService) activeProjectName() (string, error) {
	activeID, err := s.store.GetConfig("active_project_id")
	if err != nil {
		activeID = ""
	}
	projects, err := s.store.ListProjects()
	if err != nil {
		return "", err
	}
	if len(projects) == 0 {
		return "", errors.New("当前没有可导出的项目")
	}
	activeID = strings.TrimSpace(activeID)
	if activeID != "" {
		for _, project := range projects {
			if project.ID == activeID {
				return project.Name, nil
			}
		}
	}
	return projects[0].Name, nil
}

func parseSoloProjectExportStats(out []byte) soloProjectXlsxExportStats {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var stats soloProjectXlsxExportStats
		if err := json.Unmarshal([]byte(line), &stats); err == nil {
			return stats
		}
	}
	return soloProjectXlsxExportStats{}
}
