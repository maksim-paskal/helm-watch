package internal_test

import (
	"testing"

	"github.com/maksim-paskal/helm-watch/internal"
)

func TestGetFlagValue(t *testing.T) {
	t.Parallel()

	app := internal.Application{
		Args: []string{"internal", "wait-for-jobs", "--release-name test-release", "--namespace=test-namespace", "--filter", "app=test,env=prod"},
	}

	tests := make(map[string]string)
	tests["release-name"] = "test-release"
	tests["namespace"] = "test-namespace"
	tests["filter"] = "app=test,env=prod"
	tests["fake"] = "default-value"

	for key, value := range tests {
		if got := app.GetFlagValue(key, "default-value"); got != value {
			t.Errorf("Expected %s, got %s", value, got)
		}
	}
}
