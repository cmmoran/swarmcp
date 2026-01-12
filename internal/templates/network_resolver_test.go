package templates

import "testing"

func TestSwarmNetworkCIDRsArgs(t *testing.T) {
	SetNetworkCIDRResolver(NetworkCIDRResolverFunc(func(name string) ([]string, error) {
		if name == "" {
			return []string{"10.0.0.0/24"}, nil
		}
		if name == "frontend" {
			return []string{"10.0.1.0/24"}, nil
		}
		return nil, nil
	}))
	t.Cleanup(func() { SetNetworkCIDRResolver(nil) })

	got, err := swarmNetworkCIDRs()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(got) != 1 || got[0] != "10.0.0.0/24" {
		t.Fatalf("unexpected result: %#v", got)
	}

	got, err = swarmNetworkCIDRs("frontend")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(got) != 1 || got[0] != "10.0.1.0/24" {
		t.Fatalf("unexpected result: %#v", got)
	}
}
