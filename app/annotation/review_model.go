package annotation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultReviewModelLabel = "Codex CLI 默认配置"

// reviewModel keeps the CLI argument separate from the cache identity. When
// Codex chooses its own model, hash the effective evaluator configuration
// without project trust entries. Codex appends a trust entry for each ephemeral
// review directory; those entries do not affect model behavior and must not
// invalidate the evaluation that just created them.
func (s *AnnotationService) reviewModel() (cliModel, label string, err error) {
	configured, err := s.store.GetConfig("annotation_review_model")
	if err != nil {
		return "", "", err
	}
	if configured = strings.TrimSpace(configured); configured != "" {
		return configured, configured, nil
	}

	codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME"))
	if codexHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", fmt.Errorf("定位 Codex 默认模型配置失败：%w", err)
		}
		codexHome = filepath.Join(home, ".codex")
	}
	configPath := filepath.Join(codexHome, "config.toml")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", defaultReviewModelLabel, nil
		}
		return "", "", fmt.Errorf("读取 Codex 默认模型配置 %s 失败：%w", configPath, err)
	}
	digest := sha256.Sum256(reviewConfigWithoutProjectTrust(raw))
	return "", "Codex CLI 默认配置 [config:" + hex.EncodeToString(digest[:]) + "]", nil
}

func reviewConfigWithoutProjectTrust(raw []byte) []byte {
	lines := strings.Split(string(raw), "\n")
	kept := make([]string, 0, len(lines))
	skipSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			header := strings.TrimSpace(strings.Trim(trimmed, "[]"))
			skipSection = header == "projects" || strings.HasPrefix(header, "projects.")
		}
		if !skipSection {
			kept = append(kept, line)
		}
	}
	return []byte(strings.TrimSpace(strings.Join(kept, "\n")))
}
