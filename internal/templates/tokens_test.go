package templates

import "testing"

func TestExpandPathTokensOmitPartition(t *testing.T) {
	scope := Scope{
		Project:    "primary",
		Deployment: "nonprod",
		Stack:      "core",
		Partition:  "",
		Service:    "ingress",
	}
	out := ExpandPathTokens("{project}/{partition}/{stack}/{service}", scope)
	if out != "primary/core/ingress" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestExpandPathTokensAbsolute(t *testing.T) {
	scope := Scope{
		Project:    "primary",
		Deployment: "nonprod",
		Partition:  "",
	}
	out := ExpandPathTokens("/srv/{partition}/data", scope)
	if out != "/srv/data" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestExpandPathTokensPartitioned(t *testing.T) {
	scope := Scope{
		Project:    "primary",
		Deployment: "nonprod",
		Partition:  "dev",
	}
	out := ExpandPathTokens("/srv/{partition}/data", scope)
	if out != "/srv/dev/data" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestExpandTokensPreservesSlashes(t *testing.T) {
	scope := Scope{
		Project:    "primary",
		Deployment: "nonprod",
		Partition:  "",
	}
	out := ExpandTokens("/srv/{partition}/data", scope)
	if out != "/srv//data" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestExpandTokensNetworkDefaults(t *testing.T) {
	scope := Scope{
		Project:          "primary",
		NetworksShared:   "primary_core,primary_dev_extras",
		NetworkEphemeral: "primary_dev_core_svc_api",
	}
	out := ExpandTokens("{networks_shared}|{network_ephemeral}", scope)
	if out != "primary_core,primary_dev_extras|primary_dev_core_svc_api" {
		t.Fatalf("unexpected output: %q", out)
	}
}
