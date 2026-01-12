package render

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/secrets"
	"github.com/cmmoran/swarmcp/internal/sliceutil"
	"github.com/cmmoran/swarmcp/internal/templates"
)

type TemplateData struct {
	Project    string
	Deployment string
	Stack      string
	Partition  string
	Service    string
}

func (t TemplateData) EscapeTemplate(input string) string {
	return templates.EscapeTemplate(input)
}

type Summary struct {
	Configs         int
	Secrets         int
	ConfigsRendered []string
	SecretsRendered []string
	Mounts          []string
	Defs            []RenderedDef
	RuntimeRefs     []RuntimeRef
	RuntimeGraph    *templates.Graph
	MissingSecrets  []string
}

type RenderedDef struct {
	Kind    string
	Scope   string
	Name    string
	Content string
	ScopeID templates.Scope
}

type RuntimeRef struct {
	FromKind string
	FromName string
	From     templates.Scope
	FuncName string
	ToKind   string
	ToName   string
	To       templates.Scope
	Missing  bool
}

const (
	LabelManaged   = "swarmcp.io/managed"
	LabelName      = "swarmcp.io/name"
	LabelHash      = "swarmcp.io/hash"
	LabelProject   = "swarmcp.io/project"
	LabelPartition = "swarmcp.io/partition"
	LabelStack     = "swarmcp.io/stack"
	LabelService   = "swarmcp.io/service"
)

func RenderProject(cfg *config.Config, store *secrets.Store, values any, partitionFilter string, allowMissing bool, infer bool) (Summary, error) {
	var summary Summary
	data := TemplateData{Project: cfg.Project.Name, Deployment: cfg.Project.Deployment}
	resolver, err := secrets.NewResolver(cfg, store, allowMissing)
	if err != nil {
		return summary, err
	}

	projectScope := withNetworkScope(cfg, templates.Scope{Project: cfg.Project.Name, Deployment: cfg.Project.Deployment})
	if err = renderDefs(cfg, resolver, values, "project", projectScope, cfg.ProjectConfigDefs(""), cfg.ProjectSecretDefs(""), data, &summary, infer); err != nil {
		return summary, err
	}

	for stackName, stack := range cfg.Stacks {
		data.Stack = stackName
		if stack.Mode == "partitioned" && len(cfg.Project.Partitions) > 0 {
			partitions := sliceutil.FilterPartition(cfg.Project.Partitions, partitionFilter)
			for _, partitionName := range partitions {
				data.Partition = partitionName
				scope := withNetworkScope(cfg, templates.Scope{Project: cfg.Project.Name, Deployment: cfg.Project.Deployment, Stack: stackName, Partition: partitionName})
				if err = renderDefs(cfg, resolver, values, "stack "+stackName+" partition "+partitionName, scope, cfg.StackConfigDefs(stackName, partitionName), cfg.StackSecretDefs(stackName, partitionName), data, &summary, infer); err != nil {
					return summary, err
				}
			}
		} else {
			data.Partition = ""
			scope := withNetworkScope(cfg, templates.Scope{Project: cfg.Project.Name, Deployment: cfg.Project.Deployment, Stack: stackName})
			if err := renderDefs(cfg, resolver, values, "stack "+stackName, scope, cfg.StackConfigDefs(stackName, ""), cfg.StackSecretDefs(stackName, ""), data, &summary, infer); err != nil {
				return summary, err
			}
		}

		stackPartitions := sliceutil.FilterPartition(cfg.StackPartitionNames(stackName), partitionFilter)
		for _, partitionName := range stackPartitions {
			data.Partition = partitionName
			scope := withNetworkScope(cfg, templates.Scope{Project: cfg.Project.Name, Deployment: cfg.Project.Deployment, Stack: stackName, Partition: partitionName})
			if err = renderDefs(cfg, resolver, values, "stack "+stackName+" partition "+partitionName, scope, cfg.StackPartitionConfigDefs(stackName, partitionName), cfg.StackPartitionSecretDefs(stackName, partitionName), data, &summary, infer); err != nil {
				return summary, err
			}
		}

		if err = renderServiceDefs(cfg, resolver, values, stackName, stack, data.Project, data.Deployment, &summary, partitionFilter, infer); err != nil {
			return summary, err
		}

		if err = collectServiceMounts(cfg, stackName, stack, data.Project, data.Deployment, resolver, values, &summary, partitionFilter, infer); err != nil {
			return summary, err
		}
	}

	if reporter, ok := resolver.(secrets.MissingReporter); ok {
		summary.MissingSecrets = reporter.Missing()
	}
	return summary, nil
}

func renderDefs(cfg *config.Config, resolver secrets.Resolver, values any, scope string, scopeID templates.Scope, configs map[string]config.ConfigDef, secrets map[string]config.SecretDef, data TemplateData, summary *Summary, infer bool) error {
	secretValue := func(scope templates.Scope, name string) (string, error) {
		if resolver == nil {
			return "", fmt.Errorf("secret_value is not available (no secrets resolver)")
		}
		return resolver.Value(scope, name)
	}

	inferredConfigs := map[string]struct{}{}
	inferredSecrets := map[string]struct{}{}
	trace := func(fromKind string, fromName string, fromScope templates.Scope) func(templates.TraceCall) {
		return func(call templates.TraceCall) {
			if summary.RuntimeGraph == nil {
				summary.RuntimeGraph = templates.NewGraph()
			}
			fromID := runtimeNodeID(fromKind, fromScope, fromName)
			toID := runtimeNodeID(call.Kind, call.To, call.Name)
			if fromID != toID {
				summary.RuntimeGraph.AddEdge(fromID, toID)
			}
			summary.RuntimeRefs = append(summary.RuntimeRefs, RuntimeRef{
				FromKind: fromKind,
				FromName: fromName,
				From:     fromScope,
				FuncName: call.Func,
				ToKind:   call.Kind,
				ToName:   call.Name,
				To:       call.To,
				Missing:  call.Missing,
			})
			if !infer || !call.Missing {
				return
			}
			switch call.Func {
			case "config_ref":
				inferredConfigs[call.Name] = struct{}{}
			case "secret_ref":
				inferredSecrets[call.Name] = struct{}{}
			}
		}
	}

	for name, def := range configs {
		if def.Source == "" {
			continue
		}
		defScope := templates.Scope{
			Project:    data.Project,
			Deployment: data.Deployment,
			Stack:      data.Stack,
			Partition:  data.Partition,
			Service:    data.Service,
		}
		iresolver := templates.NewScopeResolverWithTrace(cfg, defScope, false, infer, data, secretValue, values, trace("config", name, defScope))
		engine := templates.New(iresolver)
		rendered, err := templates.ResolveSource(def.Source, scopeID, data, engine, values, cfg.BaseDir)
		if err != nil {
			return fmt.Errorf("%s config %q: %w", scope, name, err)
		}
		summary.Configs++
		summary.ConfigsRendered = append(summary.ConfigsRendered, fmt.Sprintf("%s %q", scope, name))
		summary.Defs = append(summary.Defs, RenderedDef{
			Kind:    "config",
			Scope:   scope,
			Name:    name,
			Content: rendered,
			ScopeID: scopeID,
		})
	}

	for name, def := range secrets {
		if def.Source == "" {
			continue
		}
		defScope := templates.Scope{
			Project:    data.Project,
			Deployment: data.Deployment,
			Stack:      data.Stack,
			Partition:  data.Partition,
			Service:    data.Service,
		}
		iresolver := templates.NewScopeResolverWithTrace(cfg, defScope, true, infer, data, secretValue, values, trace("secret", name, defScope))
		engine := templates.New(iresolver)
		rendered, err := templates.ResolveSource(def.Source, scopeID, data, engine, values, cfg.BaseDir)
		if err != nil {
			return fmt.Errorf("%s secret %q: %w", scope, name, err)
		}
		summary.Secrets++
		summary.SecretsRendered = append(summary.SecretsRendered, fmt.Sprintf("%s %q", scope, name))
		summary.Defs = append(summary.Defs, RenderedDef{
			Kind:    "secret",
			Scope:   scope,
			Name:    name,
			Content: rendered,
			ScopeID: scopeID,
		})
	}

	if infer {
		configNames := make([]string, 0, len(inferredConfigs))
		for name := range inferredConfigs {
			if name == "" {
				continue
			}
			if _, ok := configs[name]; ok {
				continue
			}
			configNames = append(configNames, name)
		}
		sort.Strings(configNames)
		for _, name := range configNames {
			source := fmt.Sprintf("values#/%s", name)
			if err := renderInferredDef(cfg, resolver, values, scope, scopeID, data, "config", name, source, summary); err != nil {
				return err
			}
		}

		secretNames := make([]string, 0, len(inferredSecrets))
		for name := range inferredSecrets {
			if name == "" {
				continue
			}
			if _, ok := secrets[name]; ok {
				continue
			}
			secretNames = append(secretNames, name)
		}
		sort.Strings(secretNames)
		for _, name := range secretNames {
			source := config.DefaultSecretSource(name, "")
			if err := renderInferredDef(cfg, resolver, values, scope, scopeID, data, "secret", name, source, summary); err != nil {
				return err
			}
		}
	}

	return nil
}

func renderServiceDefs(cfg *config.Config, resolver secrets.Resolver, values any, stackName string, stack config.Stack, projectName string, deployment string, summary *Summary, partitionFilter string, infer bool) error {
	partitions := []string{""}
	if stack.Mode == "partitioned" && len(cfg.Project.Partitions) > 0 {
		partitions = sliceutil.FilterPartition(cfg.Project.Partitions, partitionFilter)
	}

	for _, partitionName := range partitions {
		services, err := cfg.StackServices(stackName, partitionName)
		if err != nil {
			return err
		}
		if len(services) == 0 {
			continue
		}
		for serviceName, service := range services {
			configDefs := serviceConfigDefs(service.Configs)
			secretDefs := serviceSecretDefs(cfg, stackName, partitionName, service.Secrets)
			if len(configDefs) == 0 && len(secretDefs) == 0 {
				continue
			}

			data := TemplateData{
				Project:    projectName,
				Deployment: deployment,
				Stack:      stackName,
				Partition:  partitionName,
				Service:    serviceName,
			}
			scopeID := templates.Scope{
				Project:    projectName,
				Deployment: deployment,
				Stack:      stackName,
				Partition:  partitionName,
				Service:    serviceName,
			}

			scope := "stack " + stackName + " service " + serviceName
			if partitionName != "" {
				scope = "stack " + stackName + " partition " + partitionName + " service " + serviceName
			}

			if err := renderDefs(cfg, resolver, values, scope, scopeID, configDefs, secretDefs, data, summary, infer); err != nil {
				return err
			}
		}
	}

	return nil
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

func serviceSecretDefs(cfg *config.Config, stackName string, partitionName string, secrets []config.SecretRef) map[string]config.SecretDef {
	defs := make(map[string]config.SecretDef)
	for _, ref := range secrets {
		if ref.Name == "" {
			continue
		}
		if ref.Source == "" {
			if secretDefExistsOutsideService(cfg, stackName, partitionName, ref.Name) {
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

func secretDefExistsOutsideService(cfg *config.Config, stackName string, partitionName string, name string) bool {
	if cfg == nil || stackName == "" {
		return false
	}
	if partitionName != "" {
		if _, ok := cfg.StackPartitionSecretDefs(stackName, partitionName)[name]; ok {
			return true
		}
	}
	if _, ok := cfg.StackSecretDefs(stackName, partitionName)[name]; ok {
		return true
	}
	if _, ok := cfg.ProjectSecretDefs(partitionName)[name]; ok {
		return true
	}
	return false
}

func mergeConfigRefs(existing []config.ConfigRef, inferred map[string]struct{}) []config.ConfigRef {
	if len(inferred) == 0 {
		return existing
	}
	out := make([]config.ConfigRef, 0, len(existing)+len(inferred))
	seen := make(map[string]struct{}, len(existing))
	for _, ref := range existing {
		if ref.Name == "" {
			continue
		}
		seen[ref.Name] = struct{}{}
		out = append(out, ref)
	}
	names := make([]string, 0, len(inferred))
	for name := range inferred {
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		out = append(out, config.ConfigRef{Name: name})
	}
	return out
}

func mergeSecretRefs(existing []config.SecretRef, inferred map[string]struct{}) []config.SecretRef {
	if len(inferred) == 0 {
		return existing
	}
	out := make([]config.SecretRef, 0, len(existing)+len(inferred))
	seen := make(map[string]struct{}, len(existing))
	for _, ref := range existing {
		if ref.Name == "" {
			continue
		}
		seen[ref.Name] = struct{}{}
		out = append(out, ref)
	}
	names := make([]string, 0, len(inferred))
	for name := range inferred {
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		out = append(out, config.SecretRef{Name: name})
	}
	return out
}

func renderInferredDef(cfg *config.Config, resolver secrets.Resolver, values any, scopeLabel string, scopeID templates.Scope, data TemplateData, kind string, name string, source string, summary *Summary) error {
	if source == "" {
		return nil
	}
	secretValue := func(scope templates.Scope, name string) (string, error) {
		if resolver == nil {
			return "", fmt.Errorf("secret_value is not available (no secrets resolver)")
		}
		return resolver.Value(scope, name)
	}
	trace := func(call templates.TraceCall) {
		if summary.RuntimeGraph == nil {
			summary.RuntimeGraph = templates.NewGraph()
		}
		fromID := runtimeNodeID(kind, scopeID, name)
		toID := runtimeNodeID(call.Kind, call.To, call.Name)
		if fromID != toID {
			summary.RuntimeGraph.AddEdge(fromID, toID)
		}
		summary.RuntimeRefs = append(summary.RuntimeRefs, RuntimeRef{
			FromKind: kind,
			FromName: name,
			From:     scopeID,
			FuncName: call.Func,
			ToKind:   call.Kind,
			ToName:   call.Name,
			To:       call.To,
			Missing:  call.Missing,
		})
	}
	allowSecrets := kind == "secret"
	iresolver := templates.NewScopeResolverWithTrace(cfg, scopeID, allowSecrets, true, data, secretValue, values, trace)
	engine := templates.New(iresolver)
	rendered, err := templates.ResolveSource(source, scopeID, data, engine, values, cfg.BaseDir)
	if err != nil {
		return fmt.Errorf("%s %s %q: %w", scopeLabel, kind, name, err)
	}
	switch kind {
	case "config":
		summary.Configs++
		summary.ConfigsRendered = append(summary.ConfigsRendered, fmt.Sprintf("%s %q", scopeLabel, name))
	case "secret":
		summary.Secrets++
		summary.SecretsRendered = append(summary.SecretsRendered, fmt.Sprintf("%s %q", scopeLabel, name))
	}
	summary.Defs = append(summary.Defs, RenderedDef{
		Kind:    kind,
		Scope:   scopeLabel,
		Name:    name,
		Content: rendered,
		ScopeID: scopeID,
	})
	return nil
}

func inferServiceRefs(cfg *config.Config, resolver secrets.Resolver, values any, scope templates.Scope, serviceName string, service config.Service, summary *Summary) (config.Service, error) {
	inferredConfigs := make(map[string]struct{})
	inferredSecrets := make(map[string]struct{})
	missingConfigs := make(map[string]struct{})
	missingSecrets := make(map[string]struct{})
	trace := func(call templates.TraceCall) {
		if summary.RuntimeGraph == nil {
			summary.RuntimeGraph = templates.NewGraph()
		}
		fromID := runtimeNodeID("service", scope, serviceName)
		toID := runtimeNodeID(call.Kind, call.To, call.Name)
		if fromID != toID {
			summary.RuntimeGraph.AddEdge(fromID, toID)
		}
		summary.RuntimeRefs = append(summary.RuntimeRefs, RuntimeRef{
			FromKind: "service",
			FromName: serviceName,
			From:     scope,
			FuncName: call.Func,
			ToKind:   call.Kind,
			ToName:   call.Name,
			To:       call.To,
			Missing:  call.Missing,
		})
		switch call.Func {
		case "config_ref":
			inferredConfigs[call.Name] = struct{}{}
			if call.Missing {
				missingConfigs[call.Name] = struct{}{}
			}
		case "secret_ref":
			inferredSecrets[call.Name] = struct{}{}
			if call.Missing {
				missingSecrets[call.Name] = struct{}{}
			}
		}
	}
	_, engine, data := NewServiceTemplateEngine(cfg, scope, values, true, trace)
	renderedService, err := RenderServiceTemplates(engine, data, service)
	if err != nil {
		return config.Service{}, err
	}
	renderedService.Configs = mergeConfigRefs(renderedService.Configs, inferredConfigs)
	renderedService.Secrets = mergeSecretRefs(renderedService.Secrets, inferredSecrets)
	extraConfigs, extraSecrets, err := InferTemplateRefDeps(cfg, scope, renderedService.Configs, renderedService.Secrets)
	if err != nil {
		return config.Service{}, err
	}
	renderedService.Configs = mergeConfigRefs(renderedService.Configs, extraConfigs)
	renderedService.Secrets = mergeSecretRefs(renderedService.Secrets, extraSecrets)
	label := scopeLabel(scope)
	for name := range missingConfigs {
		source := fmt.Sprintf("values#/%s", name)
		if err := renderInferredDef(cfg, resolver, values, label, scope, data, "config", name, source, summary); err != nil {
			return config.Service{}, err
		}
	}
	for name := range missingSecrets {
		source := config.DefaultSecretSource(name, "")
		if err := renderInferredDef(cfg, resolver, values, label, scope, data, "secret", name, source, summary); err != nil {
			return config.Service{}, err
		}
	}
	return renderedService, nil
}

func collectServiceMounts(cfg *config.Config, stackName string, stack config.Stack, projectName string, deployment string, resolver secrets.Resolver, values any, summary *Summary, partitionFilter string, infer bool) error {
	partitions := []string{""}
	if stack.Mode == "partitioned" && len(cfg.Project.Partitions) > 0 {
		partitions = sliceutil.FilterPartition(cfg.Project.Partitions, partitionFilter)
	}

	for _, partitionName := range partitions {
		services, err := cfg.StackServices(stackName, partitionName)
		if err != nil {
			return err
		}
		if len(services) == 0 {
			continue
		}
		for serviceName, service := range services {
			scope := templates.Scope{
				Project:    projectName,
				Deployment: deployment,
				Stack:      stackName,
				Partition:  partitionName,
				Service:    serviceName,
			}

			renderedService := service
			if infer {
				renderedService, err = inferServiceRefs(cfg, resolver, values, scope, serviceName, service, summary)
				if err != nil {
					return err
				}
			}

			configResolver := templates.NewScopeResolver(cfg, scope, false, infer, TemplateData{
				Project:    projectName,
				Deployment: deployment,
				Stack:      stackName,
				Partition:  partitionName,
				Service:    serviceName,
			}, nil, values)
			secretResolver := templates.NewScopeResolver(cfg, scope, true, infer, TemplateData{
				Project:    projectName,
				Deployment: deployment,
				Stack:      stackName,
				Partition:  partitionName,
				Service:    serviceName,
			}, nil, values)

			for _, mount := range renderedService.Configs {
				if mount.Name == "" {
					return fmt.Errorf("stack %q service %q: config name is required", stackName, serviceName)
				}
				defaultTarget, err := configResolver.ConfigRef(mount.Name)
				if err != nil {
					return fmt.Errorf("stack %q service %q: config %q: %w", stackName, serviceName, mount.Name, err)
				}
				target := mount.Target
				if target == "" {
					target = defaultTarget
				} else {
					target = templates.ExpandPathTokens(target, scope)
				}
				summary.Mounts = append(summary.Mounts, formatMountLine(scope, "config", mount.Name, target))
			}

			for _, mount := range renderedService.Secrets {
				if mount.Name == "" {
					return fmt.Errorf("stack %q service %q: secret name is required", stackName, serviceName)
				}
				defaultTarget, err := secretResolver.SecretRef(mount.Name)
				if err != nil {
					return fmt.Errorf("stack %q service %q: secret %q: %w", stackName, serviceName, mount.Name, err)
				}
				target := mount.Target
				if target == "" {
					target = defaultTarget
				} else {
					target = templates.ExpandPathTokens(target, scope)
				}
				summary.Mounts = append(summary.Mounts, formatMountLine(scope, "secret", mount.Name, target))
			}
		}
	}

	return nil
}

func formatMountLine(scope templates.Scope, kind, name, target string) string {
	if scope.Partition == "" {
		return fmt.Sprintf("%s %q -> %s (stack %q service %q)", kind, name, target, scope.Stack, scope.Service)
	}
	return fmt.Sprintf("%s %q -> %s (stack %q partition %q service %q)", kind, name, target, scope.Stack, scope.Partition, scope.Service)
}

func runtimeNodeID(kind string, scope templates.Scope, name string) string {
	return kind + ":" + scopeLabel(scope) + ":" + name
}

func scopeLabel(scope templates.Scope) string {
	if scope.Stack == "" {
		return "project"
	}
	if scope.Service != "" {
		if scope.Partition != "" {
			return fmt.Sprintf("stack %s partition %s service %s", scope.Stack, scope.Partition, scope.Service)
		}
		return fmt.Sprintf("stack %s service %s", scope.Stack, scope.Service)
	}
	if scope.Partition != "" {
		return fmt.Sprintf("stack %s partition %s", scope.Stack, scope.Partition)
	}
	return fmt.Sprintf("stack %s", scope.Stack)
}

func PhysicalName(logical, content string) (string, string) {
	sum := sha256.Sum256([]byte(content))
	fullHash := hex.EncodeToString(sum[:])
	name := logical + "_" + fullHash[:12]
	if len(name) > 63 {
		name = name[:63]
	}
	return name, fullHash
}

func Labels(scope templates.Scope, logical, hash string) map[string]string {
	partition := scope.Partition
	if partition == "" {
		partition = "none"
	}
	stack := scope.Stack
	if stack == "" {
		stack = "none"
	}
	service := scope.Service
	if service == "" {
		service = "none"
	}
	return map[string]string{
		LabelManaged:   "true",
		LabelName:      logical,
		LabelHash:      hash,
		LabelProject:   scope.Project,
		LabelPartition: partition,
		LabelStack:     stack,
		LabelService:   service,
	}
}

func FormatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	parts := make([]string, 0, len(labels))
	for key, value := range labels {
		parts = append(parts, fmt.Sprintf("%s=%s", key, value))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}
