package annotation

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	appcli "github.com/blueship581/pinru/app/cli"
	"github.com/blueship581/pinru/internal/store"
)

const deepSeekReviewLabel = "DeepSeek V4 Flash"

func (s *AnnotationService) reviewProvider() (appcli.DeepSeekCodexConfig, string, error) {
	providers, err := s.store.ListLLMProviders()
	if err != nil {
		return appcli.DeepSeekCodexConfig{}, "", err
	}
	var selected *store.LLMProvider
	for _, requireDefault := range []bool{true, false} {
		for i := range providers {
			if providers[i].IsDefault == requireDefault && isDeepSeekReviewAPIProvider(providers[i]) {
				selected = &providers[i]
				break
			}
		}
		if selected != nil {
			break
		}
	}
	if selected == nil {
		return appcli.DeepSeekCodexConfig{}, "", errors.New("请先在设置中添加带 API Key 的 DeepSeek V4 Flash API 提供商，AI 审核不会自动回退到 Codex 模型")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(*selected.BaseURL), "/")
	baseURL = strings.TrimSuffix(baseURL, "/v1")
	fingerprint := sha256.Sum256([]byte(strings.TrimSpace(selected.Model) + "\n" + reviewPipelineVersion + "\nhigh"))
	label := deepSeekReviewLabel + " [engine:" + hex.EncodeToString(fingerprint[:8]) + "]"
	return appcli.DeepSeekCodexConfig{
		Model:           strings.TrimSpace(selected.Model),
		BaseURL:         baseURL,
		APIKey:          strings.TrimSpace(selected.APIKey),
		ReasoningEffort: "high",
	}, label, nil
}

func isDeepSeekReviewAPIProvider(provider store.LLMProvider) bool {
	if provider.ProviderType != "openai_compatible" || strings.TrimSpace(provider.APIKey) == "" || provider.BaseURL == nil {
		return false
	}
	model := strings.ToLower(strings.TrimSpace(provider.Model))
	if model != "deepseek-v4-flash" && model != "deepseek-flash" {
		return false
	}
	baseURL := strings.ToLower(strings.TrimRight(strings.TrimSpace(*provider.BaseURL), "/"))
	return baseURL == "https://api.deepseek.com" || baseURL == "https://api.deepseek.com/v1"
}
