package discovery

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	run "google.golang.org/api/run/v2"
)

const (
	envCloudRunProject = "DEVDASHBOARD_CLOUDRUN_PROJECT"
	envCloudRunRegion  = "DEVDASHBOARD_CLOUDRUN_REGION"

	cloudRunScanTimeout = 10 * time.Second
	// cloudRunCacheTTL keeps SPA polling from listing services on every tick.
	cloudRunCacheTTL = 10 * time.Second
)

// CloudRunConfig reports whether Cloud Run discovery is configured.
// Both variables must be set together. Both empty means the source is off.
func CloudRunConfig(getenv func(string) string) (project, region string, enabled bool, err error) {
	project = strings.TrimSpace(getenv(envCloudRunProject))
	region = strings.TrimSpace(getenv(envCloudRunRegion))
	if project == "" && region == "" {
		return "", "", false, nil
	}
	if project == "" || region == "" {
		return "", "", false, fmt.Errorf("%s and %s must both be set", envCloudRunProject, envCloudRunRegion)
	}
	return project, region, true, nil
}

// cloudRunContainer is the slice of a Cloud Run container the scanner reads.
type cloudRunContainer struct {
	Name    string
	Image   string
	Command []string
	Args    []string
	Env     []string // "NAME=VALUE"
}

// cloudRunCondition is one Cloud Run reconciliation condition.
type cloudRunCondition struct {
	Type  string
	State string
}

// cloudRunService is the slice of a Cloud Run service the scanner reads.
type cloudRunService struct {
	Name       string
	CreateTime time.Time
	Conditions []cloudRunCondition
	Containers []cloudRunContainer
}

type cloudRunLister func(ctx context.Context, project, region string) ([]cloudRunService, error)

// CloudRunSource lists Cloud Run services that run a daprd sidecar.
// Sidecar HTTP stays inside the instance, so results are not probed.
type CloudRunSource struct {
	project   string
	region    string
	namespace string
	list      cloudRunLister
	clock     func() time.Time

	mu      sync.Mutex
	last    time.Time
	results []ScanResult
	lastErr error
}

// NewCloudRunSource lists services in project/region with the Cloud Run Admin API.
// namespace is the Dapr namespace stamped on each instance (empty becomes "default").
func NewCloudRunSource(project, region, namespace string) *CloudRunSource {
	if strings.TrimSpace(namespace) == "" {
		namespace = "default"
	}
	return &CloudRunSource{
		project:   project,
		region:    region,
		namespace: namespace,
		list:      listCloudRunServices,
		clock:     time.Now,
	}
}

// Scanner returns the Cloud Run scan as a discovery.Scanner.
func (s *CloudRunSource) Scanner() Scanner { return s.scan }

func (s *CloudRunSource) scan() ([]ScanResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.last.IsZero() && s.clock().Sub(s.last) < cloudRunCacheTTL {
		return cloneScanResults(s.results), s.lastErr
	}
	ctx, cancel := context.WithTimeout(context.Background(), cloudRunScanTimeout)
	defer cancel()
	services, err := s.list(ctx, s.project, s.region)
	var results []ScanResult
	if err != nil {
		err = fmt.Errorf("cloud run list services: %w", err)
	} else {
		results = servicesToScanResults(services, s.namespace)
	}
	s.last = s.clock()
	s.results, s.lastErr = results, err
	return cloneScanResults(results), err
}

func cloneScanResults(in []ScanResult) []ScanResult {
	if in == nil {
		return nil
	}
	out := make([]ScanResult, len(in))
	copy(out, in)
	return out
}

func listCloudRunServices(ctx context.Context, project, region string) ([]cloudRunService, error) {
	svc, err := run.NewService(ctx)
	if err != nil {
		return nil, err
	}
	parent := fmt.Sprintf("projects/%s/locations/%s", project, region)
	var out []cloudRunService
	err = svc.Projects.Locations.Services.List(parent).Pages(ctx, func(resp *run.GoogleCloudRunV2ListServicesResponse) error {
		for _, item := range resp.Services {
			if item == nil {
				continue
			}
			out = append(out, cloudRunServiceFromAPI(item))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func cloudRunServiceFromAPI(item *run.GoogleCloudRunV2Service) cloudRunService {
	svc := cloudRunService{Name: path.Base(item.Name)}
	if t, err := time.Parse(time.RFC3339, item.CreateTime); err == nil {
		svc.CreateTime = t
	}
	for _, c := range item.Conditions {
		if c == nil {
			continue
		}
		svc.Conditions = append(svc.Conditions, cloudRunCondition{Type: c.Type, State: c.State})
	}
	if item.Template == nil {
		return svc
	}
	for _, c := range item.Template.Containers {
		if c == nil {
			continue
		}
		cc := cloudRunContainer{
			Name:    c.Name,
			Image:   c.Image,
			Command: append([]string(nil), c.Command...),
			Args:    append([]string(nil), c.Args...),
		}
		for _, env := range c.Env {
			if env == nil || env.Name == "" {
				continue
			}
			cc.Env = append(cc.Env, env.Name+"=")
		}
		svc.Containers = append(svc.Containers, cc)
	}
	return svc
}

// servicesToScanResults keeps services that include a daprd container and a
// non-empty --app-id. Other services are skipped.
func servicesToScanResults(services []cloudRunService, namespace string) []ScanResult {
	if namespace == "" {
		namespace = "default"
	}
	out := make([]ScanResult, 0, len(services))
	for _, svc := range services {
		daprd, app, ok := splitDaprd(svc.Containers)
		if !ok {
			continue
		}
		appID := flagValue(daprd.Args, "app-id")
		if appID == "" {
			appID = flagValue(daprd.Command, "app-id")
		}
		if appID == "" {
			logger().Warn("cloud run service has daprd but no --app-id", "service", svc.Name)
			continue
		}
		args := daprd.Args
		if len(args) == 0 {
			args = daprd.Command
		}
		health, appStatus, daprdStatus := cloudRunReady(svc.Conditions)
		paths := flagValue(args, "resources-path")
		var resourcePaths []string
		if paths != "" {
			resourcePaths = []string{paths}
		}
		rt := InferRuntime(strings.Join(app.Command, " "))
		if rt == "unknown" {
			rt = InferRuntimeFromImage(app.Image)
		}
		if rt == "unknown" {
			rt = InferRuntimeFromEnv(app.Env)
		}
		out = append(out, ScanResult{
			AppID:            appID,
			HTTPPort:         atoiFlag(args, "dapr-http-port"),
			GRPCPort:         atoiFlag(args, "dapr-grpc-port"),
			AppPort:          atoiFlag(args, "app-port"),
			AppProtocol:      flagValue(args, "app-protocol"),
			Created:          svc.CreateTime,
			RunTemplate:      "cloudrun",
			ResourcePaths:    resourcePaths,
			ConfigPath:       flagValue(args, "config"),
			Command:          strings.Join(append(append([]string{}, daprd.Command...), daprd.Args...), " "),
			Source:           SourceCloudRun,
			AppContainerName: svc.Name,
			AppImage:         app.Image,
			AppRuntime:       rt,
			SidecarReachable: false,
			Namespace:        namespace,
			Label:            svc.Name,
			AppStatus:        appStatus,
			DaprdStatus:      daprdStatus,
			Health:           health,
		})
	}
	return out
}

// splitDaprd returns the daprd container and the paired app container.
func splitDaprd(containers []cloudRunContainer) (daprd, app cloudRunContainer, ok bool) {
	daprdIdx := -1
	for i, c := range containers {
		if containerIsDaprd(c) {
			daprdIdx = i
			break
		}
	}
	if daprdIdx < 0 {
		return cloudRunContainer{}, cloudRunContainer{}, false
	}
	daprd = containers[daprdIdx]
	for i, c := range containers {
		if i == daprdIdx {
			continue
		}
		app = c
		break
	}
	return daprd, app, true
}

func containerIsDaprd(c cloudRunContainer) bool {
	if c.Name == "daprd" {
		return true
	}
	for _, part := range append(append([]string{}, c.Command...), c.Args...) {
		if path.Base(part) == "daprd" {
			return true
		}
	}
	image := strings.ToLower(c.Image)
	return strings.Contains(image, "/daprd") || strings.HasSuffix(image, "daprd")
}

func cloudRunReady(conds []cloudRunCondition) (Health, string, string) {
	state := ""
	for _, c := range conds {
		if c.Type == "Ready" {
			state = c.State
			break
		}
	}
	switch state {
	case "CONDITION_SUCCEEDED":
		return HealthHealthy, StatusRunning, StatusRunning
	case "CONDITION_RECONCILING", "CONDITION_PENDING":
		return HealthStarting, StatusRunning, StatusRunning
	case "CONDITION_FAILED":
		return HealthUnhealthy, StatusStopped, StatusStopped
	default:
		return HealthUnknown, "", ""
	}
}

func flagValue(args []string, name string) string {
	eq := "--" + name + "="
	for i, a := range args {
		if strings.HasPrefix(a, eq) {
			return strings.TrimPrefix(a, eq)
		}
		if a == "--"+name && i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			return args[i+1]
		}
	}
	return ""
}

func atoiFlag(args []string, name string) int {
	v := flagValue(args, name)
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
