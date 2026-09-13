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
// Codex chooses its own model, hash the whole config conservatively: any config
// change may affect review behavior, without exposing config contents or keys.
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
	digest := sha256.Sum256(raw)
	return "", "Codex CLI 默认配置 [config:" + hex.EncodeToString(digest[:]) + "]", nil
}
