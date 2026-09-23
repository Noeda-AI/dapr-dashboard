//go:build unit

package discovery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCloudRunConfig(t *testing.T) {
	p, r, ok, err := CloudRunConfig(envFunc(nil))
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, p)
	require.Empty(t, r)

	_, _, ok, err = CloudRunConfig(envFunc(map[string]string{envCloudRunProject: "noe"}))
	require.Error(t, err)
	require.False(t, ok)

	p, r, ok, err = CloudRunConfig(envFunc(map[string]string{
		envCloudRunProject: " noe ",
		envCloudRunRegion:  "europe-west6",
	}))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "noe", p)
	require.Equal(t, "europe-west6", r)
}

func TestServicesToScanResults(t *testing.T) {
	created := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	daprdArgs := []string{
		"--resources-path", "/dapr/components",
		"--config", "/dapr/dapr-config.yaml",
		"--app-id", "companion_backend_staging",
		"--app-port", "8080",
		"--app-protocol", "http",
		"--dapr-http-port", "3500",
		"--dapr-grpc-port", "50001",
	}
	services := []cloudRunService{
		{
			Name:       "noeda-api",
			CreateTime: created,
			Conditions: []cloudRunCondition{{Type: "Ready", State: "CONDITION_SUCCEEDED"}},
			Containers: []cloudRunContainer{
				{Name: "noeda-api", Image: "europe-west6-docker.pkg.dev/noe/noeda/noeda-api:1", Command: []string{"python"}, Env: []string{"PYTHON_VERSION="}},
				{Name: "daprd", Image: "docker.io/daprio/daprd:1.18.1", Command: []string{"./daprd"}, Args: daprdArgs},
			},
		},
		{
			Name:       "noeda-ws",
			CreateTime: created,
			Conditions: []cloudRunCondition{{Type: "Ready", State: "CONDITION_RECONCILING"}},
			Containers: []cloudRunContainer{
				{Name: "noeda-ws", Image: "europe-west6-docker.pkg.dev/noe/noeda/noeda-api:1"},
				{Name: "daprd", Image: "docker.io/daprio/daprd:1.18.1", Command: []string{"./daprd"}, Args: daprdArgs},
			},
		},
		{
			Name: "noeda-actions",
			Containers: []cloudRunContainer{
				{Name: "noeda-actions", Image: "noeda-actions:1"},
			},
		},
		{
			Name: "missing-app-id",
			Conditions: []cloudRunCondition{
				{Type: "Ready", State: "CONDITION_FAILED"},
			},
			Containers: []cloudRunContainer{
				{Name: "daprd", Command: []string{"./daprd"}, Args: []string{"--dapr-http-port", "3500"}},
			},
		},
		{
			Name: "equals-flags",
			Conditions: []cloudRunCondition{
				{Type: "Ready", State: "CONDITION_SUCCEEDED"},
			},
			Containers: []cloudRunContainer{
				{Name: "app", Image: "python:3.12"},
				{Name: "sidecar", Image: "docker.io/daprio/daprd:1.18.1", Args: []string{"--app-id=noeda-workflows", "--app-port=8200", "--dapr-http-port=3500"}},
			},
		},
	}

	got := servicesToScanResults(services, "default")
	require.Len(t, got, 3)

	api := got[0]
	require.Equal(t, "companion_backend_staging", api.AppID)
	require.Equal(t, "noeda-api", api.Key())
	require.Equal(t, SourceCloudRun, api.Source)
	require.False(t, api.SidecarReachable)
	require.Equal(t, HealthHealthy, api.Health)
	require.Equal(t, StatusRunning, api.AppStatus)
	require.Equal(t, StatusRunning, api.DaprdStatus)
	require.Equal(t, 8080, api.AppPort)
	require.Equal(t, 3500, api.HTTPPort)
	require.Equal(t, 50001, api.GRPCPort)
	require.Equal(t, "http", api.AppProtocol)
	require.Equal(t, []string{"/dapr/components"}, api.ResourcePaths)
	require.Equal(t, "/dapr/dapr-config.yaml", api.ConfigPath)
	require.Equal(t, "python", api.AppRuntime)
	require.Equal(t, "default", api.Namespace)
	require.Equal(t, created, api.Created)

	ws := got[1]
	require.Equal(t, "noeda-ws", ws.Key())
	require.Equal(t, api.AppID, ws.AppID)
	require.Equal(t, HealthStarting, ws.Health)

	wf := got[2]
	require.Equal(t, "noeda-workflows", wf.AppID)
	require.Equal(t, "equals-flags", wf.Key())
	require.Equal(t, 8200, wf.AppPort)
	require.Equal(t, "python", wf.AppRuntime)
}

func TestCloudRunReadyFailed(t *testing.T) {
	h, app, daprd := cloudRunReady([]cloudRunCondition{{Type: "Ready", State: "CONDITION_FAILED"}})
	require.Equal(t, HealthUnhealthy, h)
	require.Equal(t, StatusStopped, app)
	require.Equal(t, StatusStopped, daprd)

	h, app, daprd = cloudRunReady(nil)
	require.Equal(t, HealthUnknown, h)
	require.Empty(t, app)
	require.Empty(t, daprd)
}

func TestCloudRunSourceCacheAndError(t *testing.T) {
	now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	calls := 0
	src := &CloudRunSource{
		project:   "noe",
		region:    "europe-west6",
		namespace: "default",
		clock:     func() time.Time { return now },
		list: func(context.Context, string, string) ([]cloudRunService, error) {
			calls++
			if calls == 1 {
				return nil, errors.New("permission denied")
			}
			return []cloudRunService{{
				Name:       "noeda-workflows",
				CreateTime: now,
				Conditions: []cloudRunCondition{{Type: "Ready", State: "CONDITION_SUCCEEDED"}},
				Containers: []cloudRunContainer{
					{Name: "noeda-workflows", Image: "python:3.12"},
					{Name: "daprd", Command: []string{"./daprd"}, Args: []string{"--app-id", "noeda-workflows", "--dapr-http-port", "3500"}},
				},
			}}, nil
		},
	}
	scan := src.Scanner()
	_, err := scan()
	require.Error(t, err)
	require.Contains(t, err.Error(), "permission denied")
	_, err = scan()
	require.Error(t, err)
	require.Equal(t, 1, calls)

	now = now.Add(cloudRunCacheTTL)
	got, err := scan()
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "noeda-workflows", got[0].AppID)
	require.Equal(t, 2, calls)
}
