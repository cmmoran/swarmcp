package render

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/templates"
)

type refTarget struct {
	kind  string
	scope templates.Scope
	name  string
}

// InferTemplateRefDeps returns additional config/secret refs implied by config/secret templates.
func InferTemplateRefDeps(cfg *config.Config, scope templates.Scope, configRefs []config.ConfigRef, secretRefs []config.SecretRef) (map[string]struct{}, map[string]struct{}, error) {
	if cfg == nil {
		return nil, nil, nil
	}
	if len(configRefs) == 0 && len(secretRefs) == 0 {
		return nil, nil, nil
	}

	seenConfigs := make(map[string]struct{}, len(configRefs))
	seenSecrets := make(map[string]struct{}, len(secretRefs))
	for _, ref := range configRefs {
		if ref.Name == "" {
			continue
		}
		seenConfigs[ref.Name] = struct{}{}
	}
	for _, ref := range secretRefs {
		if ref.Name == "" {
			continue
		}
		seenSecrets[ref.Name] = struct{}{}
	}

	extraConfigs := make(map[string]struct{})
	extraSecrets := make(map[string]struct{})
	queue := make([]refTarget, 0, len(seenConfigs)+len(seenSecrets))

	rootResolver := templates.NewScopeResolver(cfg, scope, true, true, TemplateData{}, nil, nil)
	for name := range seenConfigs {
		if _, defScope, ok := rootResolver.ResolveConfigWithScope(name); ok {
			queue = append(queue, refTarget{kind: "config", scope: defScope, name: name})
		}
	}
	for name := range seenSecrets {
		if _, defScope, ok := rootResolver.ResolveSecretWithScope(name); ok {
			queue = append(queue, refTarget{kind: "secret", scope: defScope, name: name})
		}
	}

	visited := make(map[string]struct{})
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		key := fmt.Sprintf("%s:%s:%s", item.kind, templates.ExpandTokens("{project}/{stack}/{partition}/{service}", item.scope), item.name)
		if _, ok := visited[key]; ok {
			continue
		}
		visited[key] = struct{}{}

		defResolver := templates.NewScopeResolver(cfg, item.scope, true, true, TemplateData{}, nil, nil)
		var source string
		var defScope templates.Scope
		switch item.kind {
		case "config":
			def, scopeID, ok := defResolver.ResolveConfigWithScope(item.name)
			if !ok {
				continue
			}
			source = def.Source
			defScope = scopeID
		case "secret":
			def, scopeID, ok := defResolver.ResolveSecretWithScope(item.name)
			if !ok {
				continue
			}
			source = def.Source
			defScope = scopeID
		default:
			continue
		}

		refs, err := templateRefsFromSource(cfg.BaseDir, defScope, source, config.LoadOptions{Offline: cfg.Offline, CacheDir: cfg.CacheDir, Debug: cfg.Debug})
		if err != nil {
			return nil, nil, err
		}
		if len(refs) == 0 {
			continue
		}
		refResolver := templates.NewScopeResolver(cfg, defScope, true, true, TemplateData{}, nil, nil)
		for _, ref := range refs {
			if ref.Dynamic || ref.Name == "" {
				continue
			}
			switch ref.FuncName {
			case "config_ref":
				if _, ok := seenConfigs[ref.Name]; ok {
					continue
				}
				seenConfigs[ref.Name] = struct{}{}
				extraConfigs[ref.Name] = struct{}{}
				if _, refScope, ok := refResolver.ResolveConfigWithScope(ref.Name); ok {
					queue = append(queue, refTarget{kind: "config", scope: refScope, name: ref.Name})
				}
			case "secret_ref":
				if _, ok := seenSecrets[ref.Name]; ok {
					continue
				}
				seenSecrets[ref.Name] = struct{}{}
				extraSecrets[ref.Name] = struct{}{}
				if _, refScope, ok := refResolver.ResolveSecretWithScope(ref.Name); ok {
					queue = append(queue, refTarget{kind: "secret", scope: refScope, name: ref.Name})
				}
			}
		}
	}

	if len(extraConfigs) == 0 && len(extraSecrets) == 0 {
		return nil, nil, nil
	}
	return extraConfigs, extraSecrets, nil
}

func templateRefsFromSource(baseDir string, scope templates.Scope, source string, opts config.LoadOptions) ([]templates.TemplateRef, error) {
	if source == "" {
		return nil, nil
	}
	if strings.HasPrefix(source, "inline:") {
		content := strings.TrimSpace(strings.TrimPrefix(source, "inline:"))
		return templates.ExtractTemplateRefs("inline", content)
	}
	if templates.IsValuesSource(source) {
		return nil, nil
	}
	templatePath := templates.ExpandSourcePathTokens(source, scope)
	basePath, _ := templates.SplitSource(templatePath)
	if baseDir != "" && !config.IsGitSource(baseDir) && !config.IsGitSource(basePath) && !filepath.IsAbs(basePath) {
		basePath = filepath.Join(baseDir, basePath)
	}
	if !templates.IsTemplateSource(basePath) {
		return nil, nil
	}
	content, err := config.ReadSourceFile(basePath, baseDir, opts)
	if err != nil {
		return nil, err
	}
	return templates.ExtractTemplateRefs(basePath, string(content))
}
