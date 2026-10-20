package providers

import (
	"fmt"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Sources map[string]model.ConfigSource

// ApplicationDefaults 保留已经完成真实验收的四家默认选择。
func ApplicationDefaults() []Config {
	return []Config{{ID: "codex", Enabled: Bool(true), Command: "codex", Model: "gpt-6.1-sol"}, {ID: "claude", Enabled: Bool(true), Command: "claude", Model: "claude-opus-5-5"}, {ID: "grok", Enabled: Bool(true), Command: "grok", Model: "grok-4.7", Reasoning: "low"}, {ID: "agy", Enabled: Bool(true), Command: "agy", Model: "gemini-3.8-flash-medium"}}
}

func Bool(value bool) *bool   { return &value }
func IsEnabled(c Config) bool { return c.Enabled == nil || *c.Enabled }
func Effective(id string, saved []Config) (Config, Sources, error) {
	var c Config
	for _, item := range ApplicationDefaults() {
		if item.ID == id {
			c = item
		}
	}
	if c.ID == "" {
		return c, nil, fmt.Errorf("不支持的 CLI：%s", id)
	}
	sources := Sources{"command": {Kind: "default"}, "model": {Kind: "default"}, "reasoning": {Kind: "default"}, "enabled": {Kind: "default"}}
	for _, item := range saved {
		if item.ID == id {
			c.Model = item.Model
			c.Reasoning = item.Reasoning
			if item.Enabled != nil {
				c.Enabled = Bool(*item.Enabled)
				sources["enabled"] = model.ConfigSource{Kind: "saved"}
			}
			sources["model"] = model.ConfigSource{Kind: "saved"}
			sources["reasoning"] = model.ConfigSource{Kind: "saved"}
		}
	}
	prefix := "TCVIEW_" + strings.ToUpper(id) + "_"
	if value, ok := os.LookupEnv(prefix + "ENABLED"); ok {
		enabled, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return c, sources, fmt.Errorf("%sENABLED 必须为 true/false 或 1/0", prefix)
		}
		c.Enabled = Bool(enabled)
		sources["enabled"] = model.ConfigSource{Kind: "environment", Env: prefix + "ENABLED"}
	}
	for _, field := range []struct {
		name, key string
		target    *string
	}{{"model", "MODEL", &c.Model}, {"reasoning", "REASONING", &c.Reasoning}} {
		if value, ok := os.LookupEnv(prefix + field.key); ok {
			*field.target = strings.TrimSpace(value)
			sources[field.name] = model.ConfigSource{Kind: "environment", Env: prefix + field.key}
		}
	}
	return c, sources, nil
}

var selectorPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@+\[\]-]{0,199}$`)

func Validate(c Config, catalog *Catalog) error {
	if err := ValidateSyntax(c); err != nil {
		return err
	}
	if c.Reasoning == "" {
		return nil
	}
	return validateEffort(c, catalog)
}

// ValidateSyntax 用于保存停用项：保留公开参数的基本约束，但不探测未安装的 CLI 或要求其能力目录。
func ValidateSyntax(c Config) error {
	if _, _, err := Resolve(c.ID, nil); err != nil {
		return err
	}
	if c.Model != "" && !selectorPattern.MatchString(c.Model) {
		return fmt.Errorf("%s 模型 ID 仅允许 1 至 200 个字母、数字及 ._:/@+- 字符", c.ID)
	}
	if c.Reasoning == "" {
		return nil
	}
	if !selectorPattern.MatchString(c.Reasoning) {
		return fmt.Errorf("%s 推理配置含无效字符", c.ID)
	}
	return nil
}

func validateEffort(c Config, catalog *Catalog) error {
	if c.Model == "" {
		return fmt.Errorf("%s 使用 CLI 默认模型时不能确认推理档位，请同时使用 CLI 默认推理", c.ID)
	}
	if catalog != nil && catalog.Source != "cli-directory-stale" {
		for _, entry := range catalog.Models {
			if entry.ID == c.Model || entry.ResolvedModel == c.Model {
				if entry.ReasoningMode != "argument" {
					return fmt.Errorf("%s 模型 %s 的推理档位由模型 ID 编码或尚未确认，不能另传 effort", c.ID, c.Model)
				}
				for _, value := range entry.ReasoningValues {
					if value == c.Reasoning {
						return nil
					}
				}
				return fmt.Errorf("%s 模型 %s 不支持推理档位 %s", c.ID, c.Model, c.Reasoning)
			}
		}
	}
	// Known capabilities come from current CLI help/source and official model documentation.
	values := KnownEfforts(c.ID, c.Model)
	for _, v := range values {
		if v == c.Reasoning {
			return nil
		}
	}
	if catalog != nil && catalog.Source == "cli-directory-stale" {
		return fmt.Errorf("%s 无法确认模型 %s 的推理档位 %s：%s", c.ID, c.Model, c.Reasoning, catalog.Error)
	}
	return fmt.Errorf("%s 模型 %s 的推理档位 %s 尚未确认；请选已发现的组合或使用 CLI 默认推理", c.ID, c.Model, c.Reasoning)
}
func KnownEfforts(id, selector string) []string {
	switch id {
	case "grok":
		if selector == "grok-4.7" || selector == "grok-4.6" {
			return []string{"low", "medium", "high", "xhigh"}
		}
		if selector == "grok-4.5" {
			return []string{"low", "medium", "high"}
		}
	case "claude":
		for _, known := range []string{"opus", "sonnet", "claude-opus-5-5", "claude-sonnet-5-5", "claude-haiku-5-5", "claude-opus-5", "claude-sonnet-5", "claude-haiku-5", "claude-opus-4-8", "claude-opus-4-7"} {
			if selector == known {
				return []string{"low", "medium", "high", "xhigh", "max"}
			}
		}
		for _, known := range []string{"claude-opus-4-6", "claude-sonnet-4-6"} {
			if selector == known {
				return []string{"low", "medium", "high", "max"}
			}
		}

	}
	return nil
}

// RedactSensitive 只用于错误信息；从不将敏感环境值放入返回数据。
func RedactSensitive(message string) string {
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if ok && len(value) >= 4 && (strings.Contains(upper, "TOKEN") || strings.Contains(upper, "SECRET") || strings.Contains(upper, "API_KEY") || strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "AUTH")) {
			message = strings.ReplaceAll(message, value, "[已隐藏]")
		}
	}
	urlPattern := regexp.MustCompile(`https?://[^\s<>"']+`)
	message = urlPattern.ReplaceAllStringFunc(message, func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "[地址已隐藏]"
		}
		changed := false
		if parsed.User != nil {
			parsed.User = url.User("已隐藏")
			changed = true
		}
		query := parsed.Query()
		for key := range query {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "token") || strings.Contains(lower, "key") || strings.Contains(lower, "auth") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") {
				query.Set(key, "[已隐藏]")
				changed = true
			}
		}
		if changed {
			parsed.RawQuery = query.Encode()
			return parsed.String()
		}
		return raw
	})
	return message
}
