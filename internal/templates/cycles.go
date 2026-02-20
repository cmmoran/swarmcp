package templates

import (
	"fmt"
	"text/template"
	"text/template/parse"

	"github.com/Masterminds/sprig/v3"
	"github.com/cmmoran/swarmcp/internal/config"
)

func DetectCycles(cfg *config.Config, infer bool) ([]string, error) {
	graph := NewGraph()
	var warnings []string

	if err := addScopeEdges(cfg, graph, scopeKey{level: "project"}, infer, &warnings); err != nil {
		return warnings, fmt.Errorf("template cycle check: %w", err)
	}

	for stackName, stack := range cfg.Stacks {
		if err := addScopeEdges(cfg, graph, scopeKey{level: "stack", stack: stackName}, infer, &warnings); err != nil {
			return warnings, fmt.Errorf("template cycle check: %w", err)
		}
		if stack.Mode == "partitioned" {
			for _, partitionName := range cfg.Project.Partitions {
				scope := scopeKey{level: "partition", stack: stackName, partition: partitionName}
				if err := addTemplateEdges(cfg, graph, scope, cfg.StackConfigDefs(stackName, partitionName), cfg.StackSecretDefs(stackName, partitionName), infer, &warnings); err != nil {
					return warnings, fmt.Errorf("template cycle check: %w", err)
				}
			}
		}
		partitions := []string{""}
		if stack.Mode == "partitioned" {
			partitions = cfg.Project.Partitions
		}
		for _, partitionName := range partitions {
			services, err := cfg.StackServices(stackName, partitionName)
			if err != nil {
				return warnings, fmt.Errorf("template cycle check: %w", err)
			}
			if len(services) == 0 {
				continue
			}
			for serviceName := range services {
				scope := scopeKey{level: "service", stack: stackName, partition: partitionName, service: serviceName}
				if err := addScopeEdges(cfg, graph, scope, infer, &warnings); err != nil {
					return warnings, fmt.Errorf("template cycle check: %w", err)
				}
			}
		}
		for _, partitionName := range cfg.StackPartitionNames(stackName) {
			if err := addScopeEdges(cfg, graph, scopeKey{level: "partition", stack: stackName, partition: partitionName}, infer, &warnings); err != nil {
				return warnings, fmt.Errorf("template cycle check: %w", err)
			}
		}
	}

	if err := graph.DetectCycles(); err != nil {
		return warnings, fmt.Errorf("template cycle check: %w", err)
	}

	return warnings, nil
}

type scopeKey struct {
	level     string
	stack     string
	partition string
	service   string
}

func (s scopeKey) String() string {
	switch s.level {
	case "service":
		if s.partition != "" {
			return "stack " + s.stack + " partition " + s.partition + " service " + s.service
		}
		return "stack " + s.stack + " service " + s.service
	case "stack":
		return "stack " + s.stack
	case "partition":
		return "stack " + s.stack + " partition " + s.partition
	default:
		return "project"
	}
}

func addScopeEdges(cfg *config.Config, graph *Graph, scope scopeKey, infer bool, warnings *[]string) error {
	configs, secrets := scopeDefs(cfg, scope)
	return addTemplateEdges(cfg, graph, scope, configs, secrets, infer, warnings)
}

func addTemplateEdges(cfg *config.Config, graph *Graph, scope scopeKey, configs map[string]config.ConfigDef, secrets map[string]config.SecretDef, infer bool, warnings *[]string) error {
	templateScope := Scope{
		Project:    cfg.Project.Name,
		Deployment: cfg.Project.Deployment,
		Stack:      scope.stack,
		Partition:  scope.partition,
		Service:    scope.service,
	}
	opts := config.LoadOptions{Offline: cfg.Offline, CacheDir: cfg.CacheDir, Debug: cfg.Debug}

	for name, def := range configs {
		if def.Source == "" {
			continue
		}
		templatePath := ExpandSourcePathTokens(def.Source, templateScope)
		basePath, _ := SplitSource(templatePath)
		if !IsTemplateSource(basePath) {
			continue
		}
		content, err := config.ReadSourceFile(basePath, cfg.BaseDir, opts)
		if err != nil {
			return fmt.Errorf("%s config %q: %w", scope.String(), name, err)
		}

		configNode := nodeID("config", scope, name)
		graph.AddNode(configNode)

		refs, err := ExtractTemplateRefs(basePath, string(content))
		if err != nil {
			return fmt.Errorf("%s config %q: %w", scope.String(), name, err)
		}
		for _, ref := range refs {
			if ref.Kind == "secret" && ref.FuncName == "secret_value" {
				return fmt.Errorf("%s config %q: %s is not allowed in config templates", scope.String(), name, ref.FuncName)
			}
			if ref.Dynamic {
				*warnings = append(*warnings, fmt.Sprintf("%s config %q: %s has dynamic reference", scope.String(), name, ref.FuncName))
				continue
			}
			names, err := expandTemplateRefNames(cfg, scope, ref)
			if err != nil {
				return fmt.Errorf("%s config %q: %w", scope.String(), name, err)
			}
			if len(names) == 0 {
				continue
			}
			for _, resolvedName := range names {
				switch ref.Kind {
				case "config":
					refScope, _, ok := resolveConfigScope(cfg, scope, resolvedName)
					if !ok {
						if ref.FuncName == "config_value" || ref.FuncName == "config_value_index" || ref.FuncName == "config_value_get" {
							continue
						}
						if infer && ref.FuncName == "config_ref" {
							*warnings = append(*warnings, fmt.Sprintf("%s config %q: %s %q not found (inferred)", scope.String(), name, ref.FuncName, resolvedName))
							continue
						}
						return fmt.Errorf("%s config %q: %s %q not found", scope.String(), name, ref.FuncName, resolvedName)
					}
					graph.AddEdge(configNode, nodeID("config", refScope, resolvedName))
				case "secret":
					refScope, _, ok := resolveSecretScope(cfg, scope, resolvedName)
					if !ok {
						if ref.FuncName == "secret_value" {
							continue
						}
						if infer && ref.FuncName == "secret_ref" {
							*warnings = append(*warnings, fmt.Sprintf("%s config %q: %s %q not found (inferred)", scope.String(), name, ref.FuncName, resolvedName))
							continue
						}
						return fmt.Errorf("%s config %q: %s %q not found", scope.String(), name, ref.FuncName, resolvedName)
					}
					graph.AddEdge(configNode, nodeID("secret", refScope, resolvedName))
				}
			}
		}
	}

	for name, def := range secrets {
		if def.Source == "" {
			continue
		}
		templatePath := ExpandSourcePathTokens(def.Source, templateScope)
		basePath, _ := SplitSource(templatePath)
		if !IsTemplateSource(basePath) {
			continue
		}
		content, err := config.ReadSourceFile(basePath, cfg.BaseDir, opts)
		if err != nil {
			return fmt.Errorf("%s secret %q: %w", scope.String(), name, err)
		}

		secretNode := nodeID("secret", scope, name)
		graph.AddNode(secretNode)

		refs, err := ExtractTemplateRefs(basePath, string(content))
		if err != nil {
			return fmt.Errorf("%s secret %q: %w", scope.String(), name, err)
		}
		for _, ref := range refs {
			if ref.Dynamic {
				*warnings = append(*warnings, fmt.Sprintf("%s secret %q: %s has dynamic reference", scope.String(), name, ref.FuncName))
				continue
			}
			names, err := expandTemplateRefNames(cfg, scope, ref)
			if err != nil {
				return fmt.Errorf("%s secret %q: %w", scope.String(), name, err)
			}
			if len(names) == 0 {
				continue
			}
			for _, resolvedName := range names {
				switch ref.Kind {
				case "config":
					refScope, _, ok := resolveConfigScope(cfg, scope, resolvedName)
					if !ok {
						if ref.FuncName == "config_value" || ref.FuncName == "config_value_index" || ref.FuncName == "config_value_get" {
							continue
						}
						if infer && ref.FuncName == "config_ref" {
							*warnings = append(*warnings, fmt.Sprintf("%s secret %q: %s %q not found (inferred)", scope.String(), name, ref.FuncName, resolvedName))
							continue
						}
						return fmt.Errorf("%s secret %q: %s %q not found", scope.String(), name, ref.FuncName, resolvedName)
					}
					graph.AddEdge(secretNode, nodeID("config", refScope, resolvedName))
				case "secret":
					refScope, _, ok := resolveSecretScope(cfg, scope, resolvedName)
					if !ok {
						if ref.FuncName == "secret_value" {
							continue
						}
						if infer && ref.FuncName == "secret_ref" {
							*warnings = append(*warnings, fmt.Sprintf("%s secret %q: %s %q not found (inferred)", scope.String(), name, ref.FuncName, resolvedName))
							continue
						}
						return fmt.Errorf("%s secret %q: %s %q not found", scope.String(), name, ref.FuncName, resolvedName)
					}
					if !isSelfSecretRef(scope, name, refScope, resolvedName) {
						graph.AddEdge(secretNode, nodeID("secret", refScope, resolvedName))
					}
				}
			}
		}
	}

	return nil
}

func scopeDefs(cfg *config.Config, scope scopeKey) (map[string]config.ConfigDef, map[string]config.SecretDef) {
	switch scope.level {
	case "service":
		services, err := cfg.StackServices(scope.stack, scope.partition)
		if err == nil {
			if service, ok := services[scope.service]; ok {
				return serviceConfigDefs(service.Configs), serviceSecretDefs(cfg, scope, service.Secrets)
			}
		}
	case "partition":
		return cfg.StackPartitionConfigDefs(scope.stack, scope.partition), cfg.StackPartitionSecretDefs(scope.stack, scope.partition)
	case "stack":
		return cfg.StackConfigDefs(scope.stack, ""), cfg.StackSecretDefs(scope.stack, "")
	}
	return cfg.ProjectConfigDefs(""), cfg.ProjectSecretDefs("")
}

func resolveConfigScope(cfg *config.Config, scope scopeKey, name string) (scopeKey, config.ConfigDef, bool) {
	if scope.level == "service" {
		services, err := cfg.StackServices(scope.stack, scope.partition)
		if err == nil {
			if service, ok := services[scope.service]; ok {
				for _, ref := range service.Configs {
					if ref.Name != name {
						continue
					}
					if ref.Source == "" {
						continue
					}
					return scopeKey{level: "service", stack: scope.stack, partition: scope.partition, service: scope.service}, config.ConfigDef{
						Source: ref.Source,
						Target: ref.Target,
						UID:    ref.UID,
						GID:    ref.GID,
						Mode:   ref.Mode,
					}, true
				}
			}
		}
	}
	if scope.level == "service" || scope.level == "partition" {
		defs := cfg.StackPartitionConfigDefs(scope.stack, scope.partition)
		if def, ok := defs[name]; ok {
			return scopeKey{level: "partition", stack: scope.stack, partition: scope.partition}, def, true
		}
	}
	if scope.level == "service" || scope.level == "partition" || scope.level == "stack" {
		defs := cfg.StackConfigDefs(scope.stack, scope.partition)
		if def, ok := defs[name]; ok {
			return scopeKey{level: "stack", stack: scope.stack}, def, true
		}
	}
	if def, ok := cfg.ProjectConfigDefs(scope.partition)[name]; ok {
		return scopeKey{level: "project"}, def, true
	}
	return scopeKey{}, config.ConfigDef{}, false
}

func resolveSecretScope(cfg *config.Config, scope scopeKey, name string) (scopeKey, config.SecretDef, bool) {
	if scope.level == "service" {
		services, err := cfg.StackServices(scope.stack, scope.partition)
		if err == nil {
			if service, ok := services[scope.service]; ok {
				for _, ref := range service.Secrets {
					if ref.Name != name {
						continue
					}
					if ref.Source == "" {
						continue
					}
					return scopeKey{level: "service", stack: scope.stack, partition: scope.partition, service: scope.service}, config.SecretDef{
						Source: ref.Source,
						Target: ref.Target,
						UID:    ref.UID,
						GID:    ref.GID,
						Mode:   ref.Mode,
					}, true
				}
			}
		}
	}
	if scope.level == "service" || scope.level == "partition" {
		defs := cfg.StackPartitionSecretDefs(scope.stack, scope.partition)
		if def, ok := defs[name]; ok {
			return scopeKey{level: "partition", stack: scope.stack, partition: scope.partition}, def, true
		}
	}
	if scope.level == "service" || scope.level == "partition" || scope.level == "stack" {
		defs := cfg.StackSecretDefs(scope.stack, scope.partition)
		if def, ok := defs[name]; ok {
			return scopeKey{level: "stack", stack: scope.stack}, def, true
		}
	}
	if def, ok := cfg.ProjectSecretDefs(scope.partition)[name]; ok {
		return scopeKey{level: "project"}, def, true
	}
	if scope.level == "service" {
		services, err := cfg.StackServices(scope.stack, scope.partition)
		if err == nil {
			if service, ok := services[scope.service]; ok {
				for _, ref := range service.Secrets {
					if ref.Name != name {
						continue
					}
					if ref.Source != "" {
						continue
					}
					return scopeKey{level: "service", stack: scope.stack, partition: scope.partition, service: scope.service}, config.SecretDef{
						Source: config.DefaultSecretSource(ref.Name, ref.Source),
						Target: ref.Target,
						UID:    ref.UID,
						GID:    ref.GID,
						Mode:   ref.Mode,
					}, true
				}
			}
		}
	}
	return scopeKey{}, config.SecretDef{}, false
}

func nodeID(kind string, scope scopeKey, name string) string {
	return kind + ":" + scope.String() + ":" + name
}

func isSelfSecretRef(current scopeKey, name string, refScope scopeKey, refName string) bool {
	return current.String() == refScope.String() && name == refName
}

func serviceConfigDefs(configs []config.ConfigRef) map[string]config.ConfigDef {
	defs := make(map[string]config.ConfigDef)
	for _, ref := range configs {
		if ref.Name == "" {
			continue
		}
		if ref.Source == "" {
			continue
		}
		defs[ref.Name] = config.ConfigDef{
			Source: ref.Source,
			Target: ref.Target,
			UID:    ref.UID,
			GID:    ref.GID,
			Mode:   ref.Mode,
		}
	}
	return defs
}

func serviceSecretDefs(cfg *config.Config, scope scopeKey, secrets []config.SecretRef) map[string]config.SecretDef {
	defs := make(map[string]config.SecretDef)
	for _, ref := range secrets {
		if ref.Name == "" {
			continue
		}
		if ref.Source == "" {
			if secretDefExistsOutsideService(cfg, scope, ref.Name) {
				continue
			}
			ref.Source = config.DefaultSecretSource(ref.Name, ref.Source)
		}
		defs[ref.Name] = config.SecretDef{
			Source: ref.Source,
			Target: ref.Target,
			UID:    ref.UID,
			GID:    ref.GID,
			Mode:   ref.Mode,
		}
	}
	return defs
}

func secretDefExistsOutsideService(cfg *config.Config, scope scopeKey, name string) bool {
	if cfg == nil || scope.stack == "" {
		return false
	}
	if scope.partition != "" {
		if _, ok := cfg.StackPartitionSecretDefs(scope.stack, scope.partition)[name]; ok {
			return true
		}
	}
	if _, ok := cfg.StackSecretDefs(scope.stack, scope.partition)[name]; ok {
		return true
	}
	if _, ok := cfg.ProjectSecretDefs(scope.partition)[name]; ok {
		return true
	}
	return false
}

type TemplateRef struct {
	Kind     string
	FuncName string
	Name     string
	Dynamic  bool
}

func ExtractTemplateRefs(path string, content string) ([]TemplateRef, error) {
	funcs := sprig.TxtFuncMap()
	funcs["config_value"] = func(string) any { return "" }
	funcs["config_value_index"] = func(string, int) any { return "" }
	funcs["config_value_get"] = func(string, string) any { return "" }
	funcs["config_ref"] = func(string) string { return "" }
	funcs["config_refs"] = func(string) []string { return nil }
	funcs["secret_value"] = func(string) string { return "" }
	funcs["secret_ref"] = func(string) string { return "" }
	funcs["secret_refs"] = func(string) []string { return nil }
	funcs["runtime_value"] = func(args ...string) string { return "" }
	funcs["external_ip"] = func() string { return "" }
	funcs["escape_template"] = func(...string) string { return "" }
	funcs["escape_swarm_template"] = func(...string) string { return "" }
	funcs["swarm_network_cidrs"] = func(...string) []string { return nil }
	tpl, err := template.New(path).Funcs(funcs).Parse(content)
	if err != nil {
		return nil, fmt.Errorf("template cycle check parse: %w", err)
	}
	if tpl.Tree == nil || tpl.Tree.Root == nil {
		return nil, nil
	}
	var refs []TemplateRef
	walkNode(tpl.Tree.Root, func(fn string, arg parse.Node, dynamic bool) {
		ref := TemplateRef{
			FuncName: fn,
			Dynamic:  dynamic,
		}
		switch fn {
		case "config_value", "config_value_index", "config_value_get", "config_ref", "config_refs":
			ref.Kind = "config"
		case "secret_value", "secret_ref", "secret_refs":
			ref.Kind = "secret"
		}
		if !dynamic {
			if str, ok := arg.(*parse.StringNode); ok {
				ref.Name = str.Text
			}
		}
		refs = append(refs, ref)
	})
	return refs, nil
}

func walkNode(node parse.Node, onCall func(string, parse.Node, bool)) {
	switch n := node.(type) {
	case *parse.ListNode:
		for _, item := range n.Nodes {
			walkNode(item, onCall)
		}
	case *parse.ActionNode:
		walkPipe(n.Pipe, onCall)
	case *parse.IfNode:
		walkPipe(n.Pipe, onCall)
		walkNode(n.List, onCall)
		if n.ElseList != nil {
			walkNode(n.ElseList, onCall)
		}
	case *parse.RangeNode:
		walkPipe(n.Pipe, onCall)
		walkNode(n.List, onCall)
		if n.ElseList != nil {
			walkNode(n.ElseList, onCall)
		}
	case *parse.WithNode:
		walkPipe(n.Pipe, onCall)
		walkNode(n.List, onCall)
		if n.ElseList != nil {
			walkNode(n.ElseList, onCall)
		}
	case *parse.TemplateNode:
		if n.Pipe != nil {
			walkPipe(n.Pipe, onCall)
		}
	}
}

func walkPipe(pipe *parse.PipeNode, onCall func(string, parse.Node, bool)) {
	if pipe == nil {
		return
	}
	for _, cmd := range pipe.Cmds {
		walkCommand(cmd, onCall)
	}
}

func walkCommand(cmd *parse.CommandNode, onCall func(string, parse.Node, bool)) {
	if cmd == nil || len(cmd.Args) == 0 {
		return
	}
	if ident, ok := cmd.Args[0].(*parse.IdentifierNode); ok {
		switch ident.Ident {
		case "config_value", "config_value_index", "config_value_get", "config_ref", "config_refs", "secret_value", "secret_ref", "secret_refs":
			if len(cmd.Args) < 2 {
				onCall(ident.Ident, nil, true)
			} else {
				arg := cmd.Args[1]
				_, ok := arg.(*parse.StringNode)
				onCall(ident.Ident, arg, !ok)
			}
		}
	}
	for _, arg := range cmd.Args {
		if pipe, ok := arg.(*parse.PipeNode); ok {
			walkPipe(pipe, onCall)
		}
	}
}

func expandTemplateRefNames(cfg *config.Config, scope scopeKey, ref TemplateRef) ([]string, error) {
	switch ref.FuncName {
	case "config_refs":
		resolver := NewScopeResolver(cfg, Scope{
			Project:    cfg.Project.Name,
			Deployment: cfg.Project.Deployment,
			Stack:      scope.stack,
			Partition:  scope.partition,
			Service:    scope.service,
		}, true, true, nil, nil, nil)
		return resolver.ResolveConfigPattern(ref.Name)
	case "secret_refs":
		resolver := NewScopeResolver(cfg, Scope{
			Project:    cfg.Project.Name,
			Deployment: cfg.Project.Deployment,
			Stack:      scope.stack,
			Partition:  scope.partition,
			Service:    scope.service,
		}, true, true, nil, nil, nil)
		return resolver.ResolveSecretPattern(ref.Name)
	default:
		if ref.Name == "" {
			return nil, nil
		}
		return []string{ref.Name}, nil
	}
}
