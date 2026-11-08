package templateshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"virtmgr/internal/domain"
	templatesapp "virtmgr/internal/modules/templates/app"
	"virtmgr/internal/platform/authctx"

	"github.com/go-chi/chi/v5"
)

type handlerRepository struct {
	templates map[int64]domain.VMTemplate
	next      int64
}

func (r *handlerRepository) ListVMTemplates(context.Context) ([]domain.VMTemplate, error) {
	items := make([]domain.VMTemplate, 0, len(r.templates))
	for _, template := range r.templates {
		items = append(items, template)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func (r *handlerRepository) GetVMTemplate(_ context.Context, id int64) (domain.VMTemplate, error) {
	template, ok := r.templates[id]
	if !ok {
		return domain.VMTemplate{}, domain.ErrNotFound
	}
	return template, nil
}

func (r *handlerRepository) VMTemplateByName(_ context.Context, name string) (domain.VMTemplate, error) {
	for _, template := range r.templates {
		if strings.EqualFold(template.Name, name) {
			return template, nil
		}
	}
	return domain.VMTemplate{}, domain.ErrNotFound
}

func (r *handlerRepository) CreateVMTemplate(_ context.Context, spec domain.VMTemplateSpec) (domain.VMTemplate, error) {
	r.next++
	template := domain.VMTemplate{
		ID: r.next, Name: spec.Name, VCPUs: spec.VCPUs, MemoryMiB: spec.MemoryMiB,
		DiskFormat: spec.DiskFormat, Network: spec.Network, Firmware: spec.Firmware,
		SecureBoot: spec.SecureBoot, CloudInit: spec.CloudInit,
		CloudInitUserData: spec.CloudInitUserData, Autostart: spec.Autostart, Start: spec.Start,
	}
	r.templates[template.ID] = template
	return template, nil
}

func (r *handlerRepository) UpdateVMTemplate(_ context.Context, id int64, spec domain.VMTemplateSpec) (domain.VMTemplate, error) {
	if _, ok := r.templates[id]; !ok {
		return domain.VMTemplate{}, domain.ErrNotFound
	}
	template := domain.VMTemplate{
		ID: id, Name: spec.Name, VCPUs: spec.VCPUs, MemoryMiB: spec.MemoryMiB,
		DiskFormat: spec.DiskFormat, Network: spec.Network, Firmware: spec.Firmware,
		SecureBoot: spec.SecureBoot, CloudInit: spec.CloudInit,
		CloudInitUserData: spec.CloudInitUserData, Autostart: spec.Autostart, Start: spec.Start,
	}
	r.templates[id] = template
	return template, nil
}

func (r *handlerRepository) DeleteVMTemplate(_ context.Context, id int64) error {
	if _, ok := r.templates[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.templates, id)
	return nil
}

func allowAll(next http.Handler) http.Handler { return next }

func newTestRouter() *chi.Mux {
	repository := &handlerRepository{templates: map[int64]domain.VMTemplate{}}
	router := chi.NewRouter()
	New(templatesapp.New(repository, nil, false)).Mount(router, allowAll)
	return router
}

func withSession(request *http.Request) *http.Request {
	return request.WithContext(authctx.WithSession(request.Context(), domain.Session{
		User: domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin},
	}))
}

func TestTemplateRoutes(t *testing.T) {
	router := newTestRouter()
	create := withSession(httptest.NewRequest(http.MethodPost, "/templates", strings.NewReader(`{"name":"linux-base","description":"base","vcpus":2,"memoryMiB":2048,"diskFormat":"qcow2","network":"default","firmware":"uefi","secureBoot":true,"cloudInit":true,"cloudInitUserData":"#cloud-config\n","autostart":false,"start":false}`)))
	create.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, create)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var created domain.VMTemplate
	if err := json.NewDecoder(recorder.Body).Decode(&created); err != nil || created.ID != 1 {
		t.Fatalf("created = %+v, %v", created, err)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, withSession(httptest.NewRequest(http.MethodGet, "/templates", nil)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var list domain.VMTemplateList
	if err := json.NewDecoder(recorder.Body).Decode(&list); err != nil || list.Total != 1 {
		t.Fatalf("list = %+v, %v", list, err)
	}

	update := withSession(httptest.NewRequest(http.MethodPatch, "/templates/1", strings.NewReader(`{"name":"secure-web","description":"updated","vcpus":4,"memoryMiB":4096,"diskFormat":"raw","network":"default","firmware":"bios","secureBoot":false,"cloudInit":false,"autostart":true,"start":true}`)))
	update.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, update)
	if recorder.Code != http.StatusOK {
		t.Fatalf("update status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, withSession(httptest.NewRequest(http.MethodDelete, "/templates/1", nil)))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestTemplateRoutesValidateSessionAndInput(t *testing.T) {
	router := newTestRouter()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/templates", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("list without session status = %d", recorder.Code)
	}
	request := httptest.NewRequest(http.MethodPatch, "/templates/nope", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, withSession(request))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid id status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}
