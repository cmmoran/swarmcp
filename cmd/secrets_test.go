package cmd

import "testing"

func TestParseMissingSecret(t *testing.T) {
	value := "project=primary stack=core partition=dev service=api name=db_password"
	scope, ok := parseMissingSecret(value)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if scope.Project != "primary" || scope.Stack != "core" || scope.Partition != "dev" || scope.Service != "api" || scope.Name != "db_password" {
		t.Fatalf("unexpected scope: %#v", scope)
	}
	if _, ok := parseMissingSecret("project=primary name="); ok {
		t.Fatalf("expected failure for empty name")
	}
	if _, ok := parseMissingSecret("bogus"); ok {
		t.Fatalf("expected failure for invalid format")
	}
}

func TestFormatSecretsPutCommand(t *testing.T) {
	prev := opts
	t.Cleanup(func() { opts = prev })
	opts.ConfigPaths = []string{"config.yaml"}
	opts.SecretsFile = "secrets.yaml"
	opts.Deployments = []string{"prod"}

	value := "project=primary stack=core partition=dev service=api name=db_password"
	cmd, ok := formatSecretsPutCommand(value)
	if !ok {
		t.Fatalf("expected format success")
	}
	expected := "swarmcp --config config.yaml --secrets-file secrets.yaml --deployment prod secrets put db_password --stdin --stack core --partition dev --service api"
	if cmd != expected {
		t.Fatalf("unexpected command:\nexpected=%s\nactual=%s", expected, cmd)
	}
}
