package libvirtbackend

import (
	"strings"
	"testing"
)

func TestParseGuestCloudInitStatusJSON(t *testing.T) {
	result := parseGuestCloudInitStatus(`{
  "status": "error",
  "extended_status": "degraded done",
  "boot_status_code": "enabled",
  "stage": "init",
  "datasource": "DataSourceNoCloud [seed=/dev/sr0]",
  "last_update": "2026-11-16T10:20:00Z",
  "errors": ["modules-final failed"],
  "recoverable_errors": {"init-local": ["unable to apply network config"]}
}`)
	if result.State != "error" || result.Stage != "init" || len(result.Errors) != 1 {
		t.Fatalf("cloud-init result = %+v", result)
	}
	if len(result.RecoverableErrors) != 1 || !strings.Contains(result.RecoverableErrors[0], "unable to apply network config") {
		t.Fatalf("recoverable errors = %+v", result.RecoverableErrors)
	}
}

func TestParseGuestCloudInitStatusText(t *testing.T) {
	result := parseGuestCloudInitStatus("status: done\nextended_status: done\nstage: None\ndatasource: DataSourceNoCloud\nlast_update: 2026-11-16T10:20:00Z\nerrors: none\n")
	if result.State != "done" || result.Stage != "None" || result.Datasource != "DataSourceNoCloud" {
		t.Fatalf("cloud-init text result = %+v", result)
	}
	if len(result.Errors) != 0 || len(result.RecoverableErrors) != 0 {
		t.Fatalf("cloud-init errors = %+v", result)
	}
}

func TestClassifyGuestCloudInitState(t *testing.T) {
	for _, test := range []struct {
		status, extended, boot string
		errors                 []string
		want                   string
	}{
		{status: "done", want: "done"},
		{status: "running", want: "running"},
		{status: "disabled", want: "disabled"},
		{status: "not run", want: "not_run"},
		{status: "done", errors: []string{"failed module"}, want: "error"},
		{extended: "degraded done", want: "done"},
		{boot: "enabled", want: "unknown"},
	} {
		if got := classifyGuestCloudInitState(test.status, test.extended, test.boot, test.errors); got != test.want {
			t.Fatalf("classify(%q,%q,%q,%v) = %q, want %q", test.status, test.extended, test.boot, test.errors, got, test.want)
		}
	}
}
