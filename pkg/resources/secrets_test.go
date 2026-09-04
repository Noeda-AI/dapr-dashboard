//go:build unit

package resources

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/diagridio/dev-dashboard/pkg/secrets"
	"github.com/stretchr/testify/require"
)

const storeCompYAML = `apiVersion: dapr.io/v1alpha1
kind: Component
metadata:
  name: localsecretstore
spec:
  type: secretstores.local.file
  version: v1
  metadata:
  - name: secretsFile
    value: secrets.json
`

const refCompYAML = `apiVersion: dapr.io/v1alpha1
kind: Component
metadata:
  name: statestore
spec:
  type: state.redis
  version: v1
  metadata:
  - name: redisHost
    value: localhost:6379
  - name: redisPassword
    secretKeyRef:
      name: redis:password
  - name: missing
    secretKeyRef:
      name: nope
auth:
  secretStore: localsecretstore
`

func newSecretsFixture(t *testing.T) (Service, string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secretstore.yaml"), []byte(storeCompYAML), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "statestore.yaml"), []byte(refCompYAML), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secrets.json"),
		[]byte(`{"redis":{"password":"s3cr3t"}}`), 0o600))
	paths := func() []string { return []string{dir} }
	return New(paths, nil, WithSecrets(secrets.New(paths))), dir
}

func TestListCarriesSecretRefStatus(t *testing.T) {
	svc, _ := newSecretsFixture(t)

	list, err := svc.List(context.Background(), KindComponent)
	require.NoError(t, err)

	var ss Resource
	for _, r := range list {
		if r.Name == "statestore" {
			ss = r
		}
	}
	require.Len(t, ss.SecretRefs, 2)

	byField := map[string]SecretRefStatus{}
	for _, s := range ss.SecretRefs {
		byField[s.Field] = s
	}
	require.Equal(t, string(secrets.StatusResolved), byField["redisPassword"].Status)
	require.Equal(t, "localsecretstore", byField["redisPassword"].Store)
	require.Equal(t, "redis:password", byField["redisPassword"].Name)
	require.Equal(t, string(secrets.StatusKeyNotFound), byField["missing"].Status)
	require.NotEmpty(t, byField["missing"].Detail, "a failure must say what was tried")
}

// The standing convention in this repo is that secret material never crosses
// the API boundary. This is its regression guard.
func TestSecretValuesNeverSerialized(t *testing.T) {
	svc, _ := newSecretsFixture(t)

	list, err := svc.List(context.Background(), KindComponent)
	require.NoError(t, err)
	blob, err := json.Marshal(list)
	require.NoError(t, err)
	require.NotContains(t, string(blob), "s3cr3t")

	got, err := svc.Get(context.Background(), KindComponent, "statestore")
	require.NoError(t, err)
	blob, err = json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(blob), "s3cr3t")
}

func TestGetPopulatesSecretStoreInfo(t *testing.T) {
	svc, dir := newSecretsFixture(t)

	got, err := svc.Get(context.Background(), KindComponent, "localsecretstore")
	require.NoError(t, err)
	require.NotNil(t, got.SecretStore)
	require.Equal(t, "secretstores.local.file", got.SecretStore.Type)
	require.Equal(t, filepath.Join(dir, "secrets.json"), got.SecretStore.File)
	require.Equal(t, []string{"redis:password"}, got.SecretStore.Keys)
	require.Equal(t, []string{"statestore"}, got.SecretStore.UsedBy)
	require.Empty(t, got.SecretStore.InitErr)
	// contrib defaults nestedSeparator to ":" when the property is absent.
	require.Equal(t, ":", got.SecretStore.NestedSeparator)
	require.False(t, got.SecretStore.MultiValued)
}

const storeCompCustomSepYAML = `apiVersion: dapr.io/v1alpha1
kind: Component
metadata:
  name: customsecretstore
spec:
  type: secretstores.local.file
  version: v1
  metadata:
  - name: secretsFile
    value: secrets.json
  - name: nestedSeparator
    value: "|"
  - name: multiValued
    value: "true"
`

func TestSecretStoreInfoCarriesCustomSeparatorAndMultiValued(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "store.yaml"), []byte(storeCompCustomSepYAML), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secrets.json"),
		[]byte(`{"redis":{"password":"s3cr3t"}}`), 0o600))
	paths := func() []string { return []string{dir} }
	svc := New(paths, nil, WithSecrets(secrets.New(paths)))

	got, err := svc.Get(context.Background(), KindComponent, "customsecretstore")
	require.NoError(t, err)
	require.NotNil(t, got.SecretStore)
	require.Equal(t, "|", got.SecretStore.NestedSeparator)
	require.True(t, got.SecretStore.MultiValued)
}

const envStoreCompYAML = `apiVersion: dapr.io/v1alpha1
kind: Component
metadata:
  name: envsecrets
spec:
  type: secretstores.local.env
  version: v1
  metadata:
  - name: prefix
    value: MYAPP_
`

func TestEnvStoreInfoHasNoNestedSeparatorFields(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "store.yaml"), []byte(envStoreCompYAML), 0o600))
	paths := func() []string { return []string{dir} }
	svc := New(paths, nil, WithSecrets(secrets.New(paths)))

	got, err := svc.Get(context.Background(), KindComponent, "envsecrets")
	require.NoError(t, err)
	require.NotNil(t, got.SecretStore)
	require.Empty(t, got.SecretStore.NestedSeparator)
	require.False(t, got.SecretStore.MultiValued)
}

func TestNonSecretStoreHasNoStoreInfo(t *testing.T) {
	svc, _ := newSecretsFixture(t)
	got, err := svc.Get(context.Background(), KindComponent, "statestore")
	require.NoError(t, err)
	require.Nil(t, got.SecretStore)
}

func TestNilSecretsServiceIsSafe(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.yaml"), []byte(refCompYAML), 0o600))
	svc := New(func() []string { return []string{dir} }, nil) // no WithSecrets

	list, err := svc.List(context.Background(), KindComponent)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Empty(t, list[0].SecretRefs)
}
