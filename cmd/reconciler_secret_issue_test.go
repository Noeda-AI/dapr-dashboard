//go:build unit

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diagridio/dev-dashboard/pkg/secrets"
	"github.com/diagridio/dev-dashboard/pkg/statestore"
	"github.com/stretchr/testify/require"
)

// writeFixtureStore lays down a local.file secret store (secretstore.yaml)
// plus its secrets.json payload ({"redis":{"password":"v"}}) in dir, so tests
// can resolve "redis:password" via the ":"-flattened nested-key convention.
func writeFixtureStore(t *testing.T, dir string) {
	t.Helper()
	secretStoreYAML := `apiVersion: dapr.io/v1alpha1
kind: Component
metadata:
  name: localsecretstore
spec:
  type: secretstores.local.file
  metadata:
  - name: secretsFile
    value: secrets.json
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secretstore.yaml"), []byte(secretStoreYAML), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secrets.json"), []byte(`{"redis":{"password":"v"}}`), 0o600))
}

func TestResolveComponentSecretsReportsFirstIssue(t *testing.T) {
	dir := t.TempDir()
	writeFixtureStore(t, dir)
	svc := secrets.New(func() []string { return []string{dir} })

	c := statestore.Component{
		Name: "statestore", Type: "state.redis", SecretStore: "localsecretstore",
		Metadata: map[string]string{"redisHost": "localhost:6379"},
		SecretRefs: map[string]statestore.SecretRef{
			"redisPassword": {Name: "redis:password"},
			"aMissingField": {Name: "absent"},
		},
	}

	resolved, issue := resolveComponentSecrets(svc, c)
	require.Equal(t, "v", resolved["redisPassword"], "resolvable refs still apply")
	require.Contains(t, issue, "aMissingField")
	require.Contains(t, issue, "key-not-found")
}

func TestResolveComponentSecretsCleanWhenAllResolve(t *testing.T) {
	dir := t.TempDir()
	writeFixtureStore(t, dir)
	svc := secrets.New(func() []string { return []string{dir} })

	c := statestore.Component{
		Name: "statestore", Type: "state.redis", SecretStore: "localsecretstore",
		SecretRefs: map[string]statestore.SecretRef{"redisPassword": {Name: "redis:password"}},
	}
	_, issue := resolveComponentSecrets(svc, c)
	require.Empty(t, issue)
}
