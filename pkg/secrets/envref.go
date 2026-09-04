package secrets

import (
	"os"
	"strings"
)

// EnvVarAllowed mirrors dapr/pkg/runtime/processor/secret.isEnvVarAllowed:
// a denylist of empty names, APP_API_TOKEN, any DAPR_-prefixed name, and any
// name containing a space; then, when DAPR_ENV_KEYS is set (the Kubernetes
// injector sets it), a space-separated allowlist on top.
func EnvVarAllowed(key string) bool {
	upper := strings.ToUpper(key)
	switch {
	case upper == "":
		return false
	case upper == "APP_API_TOKEN":
		return false
	case strings.HasPrefix(upper, "DAPR_"):
		return false
	case strings.Contains(upper, " "):
		return false
	}

	allowlist := os.Getenv("DAPR_ENV_KEYS")
	if allowlist == "" {
		return true
	}
	for _, allowed := range strings.Split(allowlist, " ") {
		if allowed == key {
			return true
		}
	}
	return false
}

// resolveEnvRef resolves a spec.metadata[].envRef straight from the
// environment, exactly as the runtime does: no secret store, no prefix.
func resolveEnvRef(ref Ref) Result {
	detail := "env var " + ref.Name
	if !EnvVarAllowed(ref.Name) {
		return Result{Status: StatusForbidden,
			Detail: detail + " is on Dapr's denylist (DAPR_*, APP_API_TOKEN, names with spaces)"}
	}
	val := os.Getenv(ref.Name)
	if val == "" {
		return Result{Status: StatusEmptyValue, Detail: detail}
	}
	return Result{Status: StatusResolved, Value: val, Detail: detail}
}
