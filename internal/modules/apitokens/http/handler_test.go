package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"virtmgr/internal/domain"
	apitokensapp "virtmgr/internal/modules/apitokens/app"
	"virtmgr/internal/platform/authctx"
)

type stubRepository struct{}

func (stubRepository) CreateAPIToken(context.Context, int64, domain.APIToken, string) (domain.APIToken, error) {
	return domain.APIToken{}, nil
}

func (stubRepository) ListAPITokensByUser(context.Context, int64) ([]domain.APIToken, error) {
	return nil, nil
}

func (stubRepository) RevokeAPIToken(context.Context, int64, string, time.Time) error {
	return nil
}

func (stubRepository) APITokenPrincipalByHash(context.Context, string, time.Time) (domain.APITokenPrincipal, error) {
	return domain.APITokenPrincipal{}, domain.ErrUnauthorized
}

func (stubRepository) TouchAPIToken(context.Context, string, time.Time) error {
	return nil
}

func TestAPITokenEndpointsRejectTokenAuthentication(t *testing.T) {
	handler := New(apitokensapp.New(stubRepository{}, nil))
	tokenSession := domain.Session{
		User:     domain.User{ID: 7, Username: "automation", Role: domain.RoleAdmin},
		AuthMode: domain.AuthModeToken,
		TokenID:  "apitok_caller",
		Scopes:   []string{"read", "write"},
	}
	tests := []struct {
		name string
		call func(http.ResponseWriter, *http.Request)
	}{
		{name: "list", call: handler.List},
		{name: "create", call: handler.Create},
		{name: "revoke", call: handler.Revoke},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/api-tokens", nil)
			request = request.WithContext(authctx.WithSession(request.Context(), tokenSession))
			recorder := httptest.NewRecorder()
			test.call(recorder, request)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestOpenAPIContractCoversTaskCancellationAndImages(t *testing.T) {
	var document struct {
		Components struct {
			Schemas struct {
				Task struct {
					Properties map[string]any `json:"properties"`
				} `json:"Task"`
			} `json:"schemas"`
		} `json:"components"`
		Paths map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(openAPIContract, &document); err != nil {
		t.Fatalf("parse OpenAPI contract: %v", err)
	}
	for _, path := range []string{"/images", "/images/{id}", "/images/{id}/refresh", "/api-tokens", "/tasks/{id}/cancel"} {
		if _, ok := document.Paths[path]; !ok {
			t.Fatalf("OpenAPI path %q is missing", path)
		}
	}
	if _, ok := document.Components.Schemas.Task.Properties["cancellable"]; !ok {
		t.Fatal("Task schema is missing cancellable")
	}
	if _, ok := document.Components.Schemas.Task.Properties["actor"]; !ok {
		t.Fatal("Task schema is missing actor")
	}
	if _, ok := document.Components.Schemas.Task.Properties["createdAt"]; !ok {
		t.Fatal("Task schema is missing createdAt")
	}
}

func TestOpenAPIContractDescribesBridgeNetworks(t *testing.T) {
	var document struct {
		Components struct {
			Schemas map[string]struct {
				Required   []string `json:"required"`
				Properties map[string]struct {
					Enum []string `json:"enum"`
				} `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(openAPIContract, &document); err != nil {
		t.Fatalf("parse OpenAPI contract: %v", err)
	}
	create := document.Components.Schemas["NetworkCreateRequest"]
	if _, ok := create.Properties["bridge"]; !ok {
		t.Fatal("NetworkCreateRequest schema is missing bridge")
	}
	if got := create.Properties["mode"].Enum; len(got) != 3 || got[2] != "bridge" {
		t.Fatalf("NetworkCreateRequest mode enum = %v", got)
	}
	virtual := document.Components.Schemas["VirtualNetwork"]
	if _, ok := virtual.Properties["forwardMode"]; !ok {
		t.Fatal("VirtualNetwork schema is missing forwardMode")
	}
	if _, ok := document.Components.Schemas["HostBridgeList"]; !ok {
		t.Fatal("HostBridgeList schema is missing")
	}
	if _, ok := document.Components.Schemas["NetworkDiagnostics"]; !ok {
		t.Fatal("NetworkDiagnostics schema is missing")
	}
	if _, ok := virtual.Properties["natIpv6"]; !ok {
		t.Fatal("VirtualNetwork schema is missing natIpv6")
	}
	if _, ok := create.Properties["ipv6Mode"]; !ok {
		t.Fatal("NetworkCreateRequest schema is missing ipv6Mode")
	}
}

func TestOpenAPIContractCoversGuestAgentResizeConfigurationAndQoS(t *testing.T) {
	var document struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]any `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(openAPIContract, &document); err != nil {
		t.Fatalf("parse OpenAPI contract: %v", err)
	}
	operations := map[string]string{
		"/vms/{uuid}/guest-agent/diagnostics":            "get",
		"/vms/{uuid}/guest-agent/filesystem/grow":        "post",
		"/vms/{uuid}/boot":                               "patch",
		"/vms/{uuid}/cpu":                                "patch",
		"/vms/{uuid}/disks/{target}/qos":                 "patch",
		"/vms/{uuid}/network-interfaces/{mac}/qos":       "patch",
		"/storage/pools/{pool}/volumes/{volume}/convert": "post",
		"/storage/pools/{pool}/volumes/{volume}/flatten": "post",
	}
	for path, method := range operations {
		if _, ok := document.Paths[path][method]; !ok {
			t.Errorf("OpenAPI operation %s %s is missing", strings.ToUpper(method), path)
		}
	}
	for _, schema := range []string{
		"GuestAgentDiagnostics", "GuestAgentServiceLog", "GuestAgentFilesystemResizeRequest",
		"GuestAgentFilesystemResizeResult", "GuestAgentFilesystemResizeResponse", "VMBootRequest",
		"VMCPURequest", "VMDiskQoSRequest", "VMNetworkQoSRequest", "StorageVolumeConvertRequest", "StorageVolumeFlattenRequest",
		"VMNetworkModelRequest", "DatabaseRestoreStatus", "DataPlaneOperation", "DataPlaneOperationResult",
		"GuestFilesystemSampleList", "CapacityForecastList",
	} {
		if _, ok := document.Components.Schemas[schema]; !ok {
			t.Errorf("OpenAPI schema %s is missing", schema)
		}
	}
	for property, schema := range map[string]string{
		"bootDevices": "VM", "cpuModel": "VM", "cpuSockets": "VM", "cpuCores": "VM", "cpuThreads": "VM",
		"qos": "Disk", "filesystemResize": "GuestAgentCapabilities",
	} {
		if _, ok := document.Components.Schemas[schema].Properties[property]; !ok {
			t.Errorf("OpenAPI schema %s is missing property %s", schema, property)
		}
	}
}

func TestOpenAPIContractCoversMountedRoutesAndReferences(t *testing.T) {
	var document map[string]any
	if err := json.Unmarshal(openAPIContract, &document); err != nil {
		t.Fatalf("parse OpenAPI contract: %v", err)
	}
	paths, ok := document["paths"].(map[string]any)
	if !ok || len(paths) == 0 {
		t.Fatal("OpenAPI paths are missing")
	}
	requiredPaths := []string{
		"/openapi.json", "/session", "/api-tokens", "/api-tokens/{id}", "/overview",
		"/host", "/host/devices", "/host/metrics",
		"/vms", "/vms/import", "/vms/batch-actions", "/vms/{uuid}", "/vms/{uuid}/xml", "/vms/{uuid}/export",
		"/vms/{uuid}/snapshots", "/vms/{uuid}/snapshots/{snapshot}", "/vms/{uuid}/snapshots/{snapshot}/revert",
		"/vms/{uuid}/metrics", "/vms/{uuid}/clone", "/vms/{uuid}/backups", "/vms/{uuid}/iso/eject", "/vms/{uuid}/iso/attach",
		"/vms/{uuid}/autostart", "/vms/{uuid}/resources", "/vms/{uuid}/placement", "/vms/{uuid}/boot", "/vms/{uuid}/cpu",
		"/vms/{uuid}/disks", "/vms/{uuid}/disks/{target}", "/vms/{uuid}/disks/{target}/migrate", "/vms/{uuid}/disks/{target}/qos",
		"/vms/{uuid}/network-interfaces", "/vms/{uuid}/network-interfaces/{mac}", "/vms/{uuid}/network-interfaces/{mac}/qos",
		"/vms/{uuid}/devices", "/vms/{uuid}/devices/{device}",
		"/vms/{uuid}/metadata", "/vms/{uuid}/actions/{action}", "/vms/{uuid}/console-token", "/vms/{uuid}/console",
		"/vms/{uuid}/serial-console", "/vms/{uuid}/console/send-key", "/vms/{uuid}/config-versions",
		"/vms/{uuid}/config-versions/{version}", "/vms/{uuid}/config-versions/{version}/rollback",
		"/vms/{uuid}/snapshot-policies", "/snapshot-policies/{id}", "/snapshot-policies/{id}/run",
		"/vms/{uuid}/guest-agent", "/vms/{uuid}/guest-agent/actions", "/vms/{uuid}/guest-agent/channel",
		"/vms/{uuid}/guest-agent/files", "/vms/{uuid}/guest-agent/password", "/vms/{uuid}/guest-agent/ssh-keys",
		"/vms/{uuid}/guest-agent/ssh-keys/add", "/vms/{uuid}/guest-agent/ssh-keys/remove",
		"/vms/{uuid}/guest-agent/processes", "/vms/{uuid}/guest-agent/processes/signal",
		"/vms/{uuid}/guest-agent/services", "/vms/{uuid}/guest-agent/users", "/vms/{uuid}/guest-agent/packages", "/vms/{uuid}/guest-agent/policy",
		"/vms/{uuid}/guest-agent/health", "/vms/{uuid}/guest-agent/diagnostics", "/vms/{uuid}/guest-agent/filesystem/grow",
		"/storage/pools", "/storage/pools/{pool}", "/storage/pools/{pool}/actions/{action}",
		"/storage/pools/{pool}/volumes", "/storage/pools/{pool}/volumes/upload", "/storage/pools/{pool}/volumes/{volume}",
		"/storage/pools/{pool}/volumes/{volume}/convert", "/storage/pools/{pool}/volumes/{volume}/download",
		"/storage/pools/{pool}/volumes/{volume}/flatten",
		"/networks", "/networks/bridges", "/networks/{name}", "/networks/{name}/leases", "/networks/{name}/diagnostics", "/networks/{name}/hosts",
		"/networks/{name}/hosts/import", "/networks/{name}/hosts/{host}", "/networks/{name}/hosts/export",
		"/networks/{name}/actions/{action}", "/networks/{name}/port-forwards", "/networks/{name}/port-forwards/{id}",
		"/images", "/images/{id}", "/images/{id}/refresh", "/templates", "/templates/{id}",
		"/tasks", "/tasks/stream", "/tasks/{id}", "/tasks/{id}/retry", "/tasks/{id}/cancel",
		"/events", "/events/stream", "/audit-events", "/audit-events/export",
		"/users", "/users/{id}", "/users/me/password", "/users/{id}/password", "/users/{id}/sessions",
		"/users/{id}/sessions/{sessionId}", "/settings", "/settings/audit-retention",
		"/system/info", "/system/database-export", "/system/database-restore",
		"/dataplane/operations", "/dataplane/operations/{id}", "/dataplane/operations/{id}/recheck", "/dataplane/operations/{id}/unblock",
		"/backups", "/backups/summary", "/backups/import", "/backups/{id}", "/backups/{id}/export", "/backups/{id}/verify", "/backups/{id}/restore",
		"/backup-repositories", "/backup-repositories/{id}", "/backup-schedules", "/backup-schedules/{id}",
		"/backup-schedules/{id}/run", "/metrics/history", "/metrics/guest-filesystems", "/metrics/capacity-forecast", "/alert-rules", "/alert-rules/{id}",
	}
	for _, path := range requiredPaths {
		if _, exists := paths[path]; !exists {
			t.Errorf("OpenAPI path %q is missing", path)
		}
	}
	components, _ := document["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	for _, schema := range []string{"VMPlacement", "VMPlacementRequest", "GuestAgentPackage", "GuestAgentPackageInventory", "PortForward", "PortForwardRequest", "PortForwardList"} {
		if _, ok := schemas[schema]; !ok {
			t.Errorf("OpenAPI schema %q is missing", schema)
		}
	}

	operationIDs := map[string]string{}
	for path, rawItem := range paths {
		if !strings.HasPrefix(path, "/") {
			t.Errorf("path %q must start with /", path)
		}
		item, ok := rawItem.(map[string]any)
		if !ok {
			t.Errorf("path %q is not an object", path)
			continue
		}
		for method, rawOperation := range item {
			switch method {
			case "get", "post", "put", "patch", "delete", "options", "head", "trace":
			default:
				continue
			}
			operation, ok := rawOperation.(map[string]any)
			if !ok {
				t.Errorf("%s %s is not an object", method, path)
				continue
			}
			id, _ := operation["operationId"].(string)
			if strings.TrimSpace(id) == "" {
				t.Errorf("%s %s is missing operationId", method, path)
			} else if previous, duplicate := operationIDs[id]; duplicate {
				t.Errorf("operationId %q is duplicated by %s and %s %s", id, previous, method, path)
			} else {
				operationIDs[id] = method + " " + path
			}
			if _, ok := operation["summary"].(string); !ok {
				t.Errorf("%s %s is missing summary", method, path)
			}
			if tags, ok := operation["tags"].([]any); !ok || len(tags) == 0 {
				t.Errorf("%s %s is missing tags", method, path)
			}
			responses, ok := operation["responses"].(map[string]any)
			if !ok || len(responses) == 0 {
				t.Errorf("%s %s is missing responses", method, path)
			}
			assertOpenAPIPathParameters(t, document, path, item, operation)
		}
	}
	if len(operationIDs) < len(requiredPaths) {
		t.Errorf("operation count = %d, expected at least %d", len(operationIDs), len(requiredPaths))
	}
	assertOpenAPIReferences(t, document, document, "#")
}

func assertOpenAPIPathParameters(t *testing.T, document map[string]any, path string, item, operation map[string]any) {
	t.Helper()
	placeholders := map[string]struct{}{}
	for _, segment := range strings.Split(path, "/") {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			placeholders[strings.TrimSuffix(strings.TrimPrefix(segment, "{"), "}")] = struct{}{}
		}
	}
	declared := map[string]struct{}{}
	collectParameters := func(raw any) {
		parameters, _ := raw.([]any)
		for _, value := range parameters {
			parameter, ok := value.(map[string]any)
			if !ok {
				continue
			}
			if ref, ok := parameter["$ref"].(string); ok {
				parameter = resolveOpenAPIRef(document, ref)
			}
			if name, ok := parameter["name"].(string); ok {
				declared[name] = struct{}{}
			}
		}
	}
	collectParameters(item["parameters"])
	collectParameters(operation["parameters"])
	for name := range placeholders {
		if _, ok := declared[name]; !ok {
			t.Errorf("path %q is missing parameter %q", path, name)
		}
	}
}

func assertOpenAPIReferences(t *testing.T, root map[string]any, value any, location string) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		if ref, ok := typed["$ref"].(string); ok {
			if resolveOpenAPIRef(root, ref) == nil {
				t.Errorf("unresolved OpenAPI reference %q at %s", ref, location)
			}
		}
		for key, child := range typed {
			assertOpenAPIReferences(t, root, child, location+"/"+key)
		}
	case []any:
		for index, child := range typed {
			assertOpenAPIReferences(t, root, child, location+"/"+string(rune(index)))
		}
	}
}

func resolveOpenAPIRef(root map[string]any, ref string) map[string]any {
	if !strings.HasPrefix(ref, "#/") {
		return nil
	}
	var current any = root
	for _, encoded := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		key := strings.ReplaceAll(strings.ReplaceAll(encoded, "~1", "/"), "~0", "~")
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = object[key]
		if !ok {
			return nil
		}
	}
	resolved, _ := current.(map[string]any)
	return resolved
}
