package config

import "testing"

func TestLayeredPolicyForPath(t *testing.T) {
	cases := []struct {
		name string
		path []string
		want layeredPolicyAction
	}{
		{
			name: "merge by default",
			path: []string{"stacks", "core", "services", "api", "env"},
			want: layeredPolicyMerge,
		},
		{
			name: "replace image",
			path: []string{"stacks", "core", "services", "api", "image"},
			want: layeredPolicyReplace,
		},
		{
			name: "invalid import overrides",
			path: []string{"stacks", "core", "overrides"},
			want: layeredPolicyInvalid,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := layeredPolicyForPath(tc.path); got != tc.want {
				t.Fatalf("layeredPolicyForPath(%v) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestReleasePolicyTree(t *testing.T) {
	stacks := releasePolicyChild(releasePolicyRoot, "stacks")
	if stacks == nil {
		t.Fatalf("expected stacks policy")
	}
	stack := releasePolicyChild(stacks, "core")
	if stack == nil || !stack.requireExistingMap {
		t.Fatalf("expected existing stack policy, got %#v", stack)
	}
	services := releasePolicyChild(stack, "services")
	if services == nil || services.kind != releaseValueMap {
		t.Fatalf("expected services map policy, got %#v", services)
	}
	service := releasePolicyChild(services, "api")
	if service == nil || !service.requireExistingMap {
		t.Fatalf("expected existing service policy, got %#v", service)
	}
	updateConfig := releasePolicyChild(service, "update_config")
	if updateConfig == nil || updateConfig.kind != releaseValueUpdatePolicyMap {
		t.Fatalf("expected update_config policy, got %#v", updateConfig)
	}
	if child := releasePolicyChild(service, "ports"); child != nil {
		t.Fatalf("expected ports to be disallowed, got %#v", child)
	}
}
