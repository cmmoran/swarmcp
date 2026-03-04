package cmd

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/cmmoran/swarmcp/internal/cmdutil"
	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/spf13/cobra"
)

type sourceEntry struct {
	Key    string
	Kind   string
	URL    string
	Ref    string
	Path   string
	Local  string
	Origin string
}

var sourcesCmd = &cobra.Command{
	Use:   "sources",
	Short: "Manage external config sources",
	RunE: func(cmd *cobra.Command, args []string) error {
		return sourcesViewCmd.RunE(cmd, args)
	},
}

var sourcesViewCmd = &cobra.Command{
	Use:   "view",
	Short: "List external sources referenced by the project",
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := loadSourceEntries(cmd)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		if len(entries) == 0 {
			_, _ = fmt.Fprintln(out, "sources: none")
			return nil
		}
		for i, entry := range entries {
			_, _ = fmt.Fprintf(out, "%d) %s\n", i+1, formatSourceEntry(entry))
		}
		return nil
	},
}

var sourcesPullCmd = &cobra.Command{
	Use:   "pull [source]",
	Short: "Fetch external git sources",
	RunE: func(cmd *cobra.Command, args []string) error {
		if opts.Offline {
			return fmt.Errorf("offline mode is enabled; disable --offline to pull sources")
		}
		entries, err := loadSourceEntries(cmd)
		if err != nil {
			return err
		}
		selected, err := selectSources(entries, args)
		if err != nil {
			return err
		}
		if len(selected) == 0 {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "sources: none")
			return nil
		}
		loadOpts, err := sourcesLoadOptions()
		if err != nil {
			return err
		}
		for _, entry := range selected {
			if entry.Kind != "git" {
				continue
			}
			if _, err := config.FetchGitSource(entry.URL, entry.Ref, entry.Path, loadOpts); err != nil {
				return err
			}
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "sources pulled")
		return nil
	},
}

var sourcesDiffCmd = &cobra.Command{
	Use:   "diff [source]",
	Short: "Fetch and diff external git sources",
	RunE: func(cmd *cobra.Command, args []string) error {
		if opts.Offline {
			return fmt.Errorf("offline mode is enabled; disable --offline to diff sources")
		}
		entries, err := loadSourceEntries(cmd)
		if err != nil {
			return err
		}
		selected, err := selectSources(entries, args)
		if err != nil {
			return err
		}
		if len(selected) == 0 {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "sources: none")
			return nil
		}
		loadOpts, err := sourcesLoadOptions()
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		for _, entry := range selected {
			if entry.Kind != "git" {
				_, _ = fmt.Fprintf(out, "%s: skipped (local source)\n", entry.Key)
				continue
			}
			before, _, _ := config.ReadSourceMetadata(entry.URL, entry.Ref, entry.Path, loadOpts)
			after, err := config.FetchGitSource(entry.URL, entry.Ref, entry.Path, loadOpts)
			if err != nil {
				return err
			}
			if before.Commit == "" {
				_, _ = fmt.Fprintf(out, "%s: new commit=%s subtree=%s\n", entry.Key, after.Commit, after.Subtree)
				continue
			}
			if before.Commit == after.Commit && before.Subtree == after.Subtree {
				_, _ = fmt.Fprintf(out, "%s: no changes (commit=%s)\n", entry.Key, after.Commit)
				continue
			}
			_, _ = fmt.Fprintf(out, "%s: commit %s -> %s subtree %s -> %s\n", entry.Key, before.Commit, after.Commit, before.Subtree, after.Subtree)
		}
		return nil
	},
}

func init() {
	sourcesCmd.AddCommand(sourcesViewCmd)
	sourcesCmd.AddCommand(sourcesPullCmd)
	sourcesCmd.AddCommand(sourcesDiffCmd)
	rootCmd.AddCommand(sourcesCmd)
}

func loadSourceEntries(cmd *cobra.Command) ([]sourceEntry, error) {
	deployment, err := singleSelector("deployment", opts.Deployments)
	if err != nil {
		return nil, err
	}
	partition, err := singleSelector("partition", opts.Partitions)
	if err != nil {
		return nil, err
	}
	ctx, err := cmdutil.LoadProjectContext(cmdutil.ProjectOptions{
		ConfigPath:  opts.ConfigPath,
		SecretsFile: opts.SecretsFile,
		ValuesFiles: opts.ValuesFiles,
		Deployment:  deployment,
		Context:     opts.Context,
		Partition:   partition,
		Offline:     opts.Offline,
		Debug:       opts.Debug,
	}, false, false)
	if err != nil {
		return nil, err
	}
	entries := gatherSourceEntries(ctx.Config)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Key < entries[j].Key
	})
	return entries, nil
}

func sourcesLoadOptions() (config.LoadOptions, error) {
	cfg, err := config.LoadWithOptions(opts.ConfigPath, config.LoadOptions{Offline: opts.Offline, Debug: opts.Debug})
	if err != nil {
		return config.LoadOptions{}, err
	}
	return config.LoadOptions{CacheDir: cfg.CacheDir, Offline: opts.Offline, Debug: opts.Debug}, nil
}

func gatherSourceEntries(cfg *config.Config) []sourceEntry {
	entries := make(map[string]sourceEntry)
	addEntry := func(entry sourceEntry) {
		if entry.Key == "" {
			return
		}
		if _, ok := entries[entry.Key]; ok {
			return
		}
		entries[entry.Key] = entry
	}

	addSourcePath := func(source string, origin string) {
		if source == "" {
			return
		}
		if strings.HasPrefix(source, "inline:") || strings.HasPrefix(source, "values#") {
			return
		}
		if config.IsGitSource(source) {
			parsed, ok, err := config.ParseGitSource(source)
			if err == nil && ok {
				addEntry(sourceEntry{
					Key:    formatGitKey(parsed.URL, parsed.Ref, parsed.Path),
					Kind:   "git",
					URL:    parsed.URL,
					Ref:    parsed.Ref,
					Path:   parsed.Path,
					Origin: origin,
				})
			}
			return
		}
		if filepath.IsAbs(source) {
			addEntry(sourceEntry{Key: source, Kind: "dir", Local: source, Origin: origin})
		}
	}

	addSourcesRoot := func(s config.Sources, origin string) {
		if s.URL == "" && s.Path == "" {
			return
		}
		root, err := config.ResolveSourcesRoot(s, config.LoadOptions{CacheDir: cfg.CacheDir, Offline: opts.Offline, Debug: opts.Debug})
		if err != nil {
			return
		}
		if config.IsGitSource(root) {
			parsed, ok, err := config.ParseGitSource(root)
			if err == nil && ok {
				addEntry(sourceEntry{
					Key:    formatGitKey(parsed.URL, parsed.Ref, parsed.Path),
					Kind:   "git",
					URL:    parsed.URL,
					Ref:    parsed.Ref,
					Path:   parsed.Path,
					Origin: origin,
				})
			}
			return
		}
		if filepath.IsAbs(root) {
			addEntry(sourceEntry{Key: root, Kind: "dir", Local: root, Origin: origin})
		}
	}

	if cfg == nil {
		return nil
	}

	addSourcesRoot(cfg.Project.Sources, "project.sources")
	if config.IsGitSource(cfg.BaseDir) {
		addSourcePath(cfg.BaseDir, "project.base")
	}
	for stackName, stack := range cfg.Stacks {
		addSourcesRoot(stack.Sources, "stack."+stackName+".sources")
		if config.IsGitSource(stack.BaseDir) {
			addSourcePath(stack.BaseDir, "stack."+stackName+".base")
		}
		for partitionName, partition := range stack.Partitions {
			addSourcesRoot(partition.Sources, "stack."+stackName+".partition."+partitionName+".sources")
		}
		for serviceName, service := range stack.Services {
			addSourcesRoot(service.Sources, "stack."+stackName+".service."+serviceName+".sources")
			if config.IsGitSource(service.BaseDir) {
				addSourcePath(service.BaseDir, "stack."+stackName+".service."+serviceName+".base")
			}
		}
		for name, def := range stack.Configs.Defs {
			addSourcePath(def.Source, "stack."+stackName+".configs."+name)
		}
		for name, def := range stack.Secrets.Defs {
			addSourcePath(def.Source, "stack."+stackName+".secrets."+name)
		}
		for overlayName, overlay := range stack.Overlays.Deployments {
			addSourcesRoot(overlay.Sources, "stack."+stackName+".overlays.deployments."+overlayName+".sources")
			for partitionName, partition := range overlay.Partitions {
				addSourcesRoot(partition.Sources, "stack."+stackName+".overlays.deployments."+overlayName+".partitions."+partitionName+".sources")
			}
		}
		for _, overlay := range stack.Overlays.Partitions.Rules {
			overlayName := overlay.Name
			addSourcesRoot(overlay.Sources, "stack."+stackName+".overlays.partitions."+overlayName+".sources")
			for partitionName, partition := range overlay.Partitions {
				addSourcesRoot(partition.Sources, "stack."+stackName+".overlays.partitions."+overlayName+".partitions."+partitionName+".sources")
			}
		}
	}
	for name, def := range cfg.Project.Configs {
		addSourcePath(def.Source, "project.configs."+name)
	}
	for name, def := range cfg.Project.Secrets {
		addSourcePath(def.Source, "project.secrets."+name)
	}
	for overlayName, overlay := range cfg.Overlays.Deployments {
		addSourcesRoot(overlay.Project.Sources, "overlays.deployments."+overlayName+".project.sources")
		for stackName, stack := range overlay.Stacks {
			addSourcesRoot(stack.Sources, "overlays.deployments."+overlayName+".stack."+stackName+".sources")
			for partitionName, partition := range stack.Partitions {
				addSourcesRoot(partition.Sources, "overlays.deployments."+overlayName+".stack."+stackName+".partition."+partitionName+".sources")
			}
		}
	}
	for _, overlay := range cfg.Overlays.Partitions.Rules {
		overlayName := overlay.Name
		addSourcesRoot(overlay.Project.Sources, "overlays.partitions."+overlayName+".project.sources")
		for stackName, stack := range overlay.Stacks {
			addSourcesRoot(stack.Sources, "overlays.partitions."+overlayName+".stack."+stackName+".sources")
			for partitionName, partition := range stack.Partitions {
				addSourcesRoot(partition.Sources, "overlays.partitions."+overlayName+".stack."+stackName+".partition."+partitionName+".sources")
			}
		}
	}

	out := make([]sourceEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry)
	}
	return out
}

func formatSourceEntry(entry sourceEntry) string {
	switch entry.Kind {
	case "git":
		return fmt.Sprintf("git url=%s ref=%s path=%s", entry.URL, entry.Ref, entry.Path)
	case "dir":
		return fmt.Sprintf("dir %s", entry.Local)
	default:
		return entry.Key
	}
}

func formatGitKey(url, ref, path string) string {
	return fmt.Sprintf("git:%s@%s#%s", url, ref, path)
}

func selectSources(entries []sourceEntry, args []string) ([]sourceEntry, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	if len(args) == 0 {
		return entries, nil
	}
	if len(args) > 1 {
		return nil, fmt.Errorf("expected a single source identifier")
	}
	arg := strings.TrimSpace(args[0])
	if arg == "" || strings.EqualFold(arg, "all") {
		return entries, nil
	}
	if idx, err := strconv.Atoi(arg); err == nil {
		if idx < 1 || idx > len(entries) {
			return nil, fmt.Errorf("source index out of range: %d", idx)
		}
		return []sourceEntry{entries[idx-1]}, nil
	}
	for _, entry := range entries {
		if entry.Key == arg {
			return []sourceEntry{entry}, nil
		}
	}
	matches := make([]sourceEntry, 0)
	for _, entry := range entries {
		if strings.Contains(entry.Key, arg) || strings.Contains(entry.URL, arg) || strings.Contains(entry.Path, arg) {
			matches = append(matches, entry)
		}
	}
	if len(matches) == 1 {
		return matches, nil
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("source not found: %s", arg)
	}
	return nil, fmt.Errorf("source selector %q matched %d entries", arg, len(matches))
}
