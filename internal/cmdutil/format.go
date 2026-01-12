package cmdutil

import (
	"fmt"

	"github.com/cmmoran/swarmcp/internal/templates"
)

func ServiceScopeLabel(stack, partition, service string) string {
	if partition == "" {
		return fmt.Sprintf("%s/%s", stack, service)
	}
	return fmt.Sprintf("%s/%s/%s", stack, partition, service)
}

func ScopeLabel(scope templates.Scope) string {
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
