package config

type layeredPolicyAction int

const (
	layeredPolicyMerge layeredPolicyAction = iota
	layeredPolicyReplace
	layeredPolicyInvalid
)

type layeredPolicyRule struct {
	pattern []string
	action  layeredPolicyAction
}

var layeredPolicyRules = []layeredPolicyRule{
	{pattern: []string{"stacks", "*", "overrides"}, action: layeredPolicyInvalid},
	{pattern: []string{"stacks", "*", "services", "*", "overrides"}, action: layeredPolicyInvalid},

	{pattern: []string{"project", "name"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "deployment"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "restart_policy"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "update_config"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "rollback_config"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "secrets_engine"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "preserve_unused_resources"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "partitions"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "deployments"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "defaults", "networks", "shared"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "defaults", "networks", "internal"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "defaults", "networks", "egress"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "defaults", "networks", "attachable"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "defaults", "volumes", "driver"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "defaults", "volumes", "base_path"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "defaults", "volumes", "layout"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "defaults", "volumes", "node_label_key"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "defaults", "volumes", "service_standard"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "defaults", "volumes", "service_target"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "nodes", "*", "roles"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "nodes", "*", "volumes"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "deployment_targets", "*", "include", "names"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "deployment_targets", "*", "exclude", "names"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "deployment_targets", "*", "overrides", "*", "roles"}, action: layeredPolicyReplace},
	{pattern: []string{"project", "deployment_targets", "*", "overrides", "*", "volumes"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "source"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "mode"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "restart_policy"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "update_config"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "rollback_config"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "partitions", "*", "restart_policy"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "partitions", "*", "update_config"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "partitions", "*", "rollback_config"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "services", "*", "source"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "services", "*", "image"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "services", "*", "command"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "services", "*", "args"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "services", "*", "workdir"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "services", "*", "ports"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "services", "*", "mode"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "services", "*", "replicas"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "services", "*", "restart_policy"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "services", "*", "update_config"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "services", "*", "rollback_config"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "services", "*", "healthcheck"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "services", "*", "depends_on"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "services", "*", "egress"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "services", "*", "networks"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "services", "*", "network_ephemeral"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "services", "*", "configs"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "services", "*", "secrets"}, action: layeredPolicyReplace},
	{pattern: []string{"stacks", "*", "services", "*", "volumes"}, action: layeredPolicyReplace},
}

type releaseValueKind int

const (
	releaseValueMap releaseValueKind = iota
	releaseValueScalar
	releaseValueScalarMap
	releaseValueUpdatePolicyMap
)

type releasePolicyNode struct {
	segment            string
	requireExistingMap bool
	kind               releaseValueKind
	children           []*releasePolicyNode
}

var releasePolicyRoot = &releasePolicyNode{
	kind: releaseValueMap,
	children: []*releasePolicyNode{
		{
			segment: "stacks",
			kind:    releaseValueMap,
			children: []*releasePolicyNode{
				{
					segment:            "*",
					requireExistingMap: true,
					kind:               releaseValueMap,
					children: []*releasePolicyNode{
						{
							segment:            "source",
							requireExistingMap: true,
							kind:               releaseValueMap,
							children: []*releasePolicyNode{
								{segment: "ref", kind: releaseValueScalar},
							},
						},
						{
							segment: "services",
							kind:    releaseValueMap,
							children: []*releasePolicyNode{
								{
									segment:            "*",
									requireExistingMap: true,
									kind:               releaseValueMap,
									children: []*releasePolicyNode{
										{segment: "image", kind: releaseValueScalar},
										{segment: "replicas", kind: releaseValueScalar},
										{segment: "env", kind: releaseValueScalarMap},
										{segment: "labels", kind: releaseValueScalarMap},
										{segment: "update_config", kind: releaseValueUpdatePolicyMap},
										{segment: "rollback_config", kind: releaseValueUpdatePolicyMap},
									},
								},
							},
						},
					},
				},
			},
		},
	},
}

func layeredPolicyForPath(path []string) layeredPolicyAction {
	for _, rule := range layeredPolicyRules {
		if pathMatches(path, rule.pattern...) {
			return rule.action
		}
	}
	return layeredPolicyMerge
}

func releasePolicyChild(node *releasePolicyNode, segment string) *releasePolicyNode {
	for _, child := range node.children {
		if child.segment == segment || child.segment == "*" {
			return child
		}
	}
	return nil
}
