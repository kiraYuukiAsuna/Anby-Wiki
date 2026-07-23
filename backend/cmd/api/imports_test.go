package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/importer"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/internal/platform/storage"
	"github.com/anby/wiki/backend/testkit"
)

func TestImportAPI_CreateDetailOwnershipCancelAndRetry(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	owner := tdb.MakeActor(t, "human", "import owner")
	other := tdb.MakeActor(t, "human", "other actor")
	repository := importer.NewRepository(tdb.Pool)
	api := NewImportAPI(importer.NewService(repository, db.NewTxManager(tdb.Pool), id.NewGenerator()))
	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)),
		Deps{Service: "wiki-api", Version: "test"}, nil, nil, nil, nil, nil, api)
	headers := map[string]string{"X-Actor-ID": owner.String(), "Idempotency-Key": "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4c01"}
	status, created := doJSON(t, router, http.MethodPost, "/api/v1/import-jobs",
		map[string]any{"job_type": "source_import", "config": map[string]any{"url": "https://example.com/source"}}, headers)
	if status != http.StatusCreated || created["status"] != importer.JobQueued {
		t.Fatalf("create status=%d body=%v", status, created)
	}
	id := created["id"].(string)
	status, detail := doJSON(t, router, http.MethodGet, "/api/v1/import-jobs/"+id, nil,
		map[string]string{"X-Actor-ID": owner.String()})
	if status != http.StatusOK || detail["job"].(map[string]any)["id"] != id {
		t.Fatalf("detail status=%d body=%v", status, detail)
	}
	status, forbidden := doJSON(t, router, http.MethodGet, "/api/v1/import-jobs/"+id, nil,
		map[string]string{"X-Actor-ID": other.String()})
	if status != http.StatusForbidden || forbidden["code"] != "forbidden" {
		t.Fatalf("ownership status=%d body=%v", status, forbidden)
	}
	status, cancelled := doJSON(t, router, http.MethodPost, "/api/v1/import-jobs/"+id+"/cancel", nil,
		map[string]string{"X-Actor-ID": owner.String()})
	if status != http.StatusOK || cancelled["job"].(map[string]any)["status"] != importer.JobCancelled {
		t.Fatalf("cancel status=%d body=%v", status, cancelled)
	}
	status, retried := doJSON(t, router, http.MethodPost, "/api/v1/import-jobs/"+id+"/retry", nil,
		map[string]string{"X-Actor-ID": owner.String()})
	if status != http.StatusOK || retried["job"].(map[string]any)["status"] != importer.JobQueued {
		t.Fatalf("retry status=%d body=%v", status, retried)
	}
}

func TestImportAPI_UploadStagesPrivateAssetAndQueuesJob(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	actor := tdb.MakeActor(t, "human", "upload owner")
	ids := id.NewGenerator()
	txm := db.NewTxManager(tdb.Pool)
	pageRepo := page.NewRepository(tdb.Pool)
	objectStore := storage.NewFake()
	evidenceService := evidence.NewService(evidence.NewRepository(tdb.Pool), pageRepo, objectStore, "test", txm, ids)
	jobs := importer.NewService(importer.NewRepository(tdb.Pool), txm, ids)
	api := NewImportAPI(jobs).WithUploads(evidenceService, testkit.DefaultWikiID)
	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)),
		Deps{Service: "wiki-api", Version: "test"}, nil, nil, nil, nil, nil, api)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="release.html"`)
	header.Set("Content-Type", "text/html")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("<!doctype html><html><body>verified release</body></html>"))
	_ = writer.WriteField("title", "Release upload")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/import-jobs/uploads", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("X-Actor-ID", actor.String())
	request.Header.Set("Idempotency-Key", "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4c02")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var job importer.Job
	if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	var config importer.SourceImportConfig
	if err := json.Unmarshal(job.Config, &config); err != nil || config.Source.Kind != "upload" ||
		config.Source.Filename != "release.html" || config.Source.ContentHash == "" {
		t.Fatalf("config=%+v err=%v", config, err)
	}
	if keys := objectStore.Keys(); len(keys) != 1 || keys[0] != config.Source.StorageKey {
		t.Fatalf("keys=%v config=%+v", keys, config.Source)
	}
}

func TestImportAPI_UploadRejectsUntrustedBrowserWithoutStoringAsset(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ids := id.NewGenerator()
	txm := db.NewTxManager(tdb.Pool)
	objectStore := storage.NewFake()
	evidenceService := evidence.NewService(evidence.NewRepository(tdb.Pool), page.NewRepository(tdb.Pool),
		objectStore, "test", txm, ids)
	api := NewImportAPI(importer.NewService(importer.NewRepository(tdb.Pool), txm, ids)).
		WithUploads(evidenceService, testkit.DefaultWikiID)
	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)),
		Deps{
			Service: "wiki-api", Version: "test", SessionCookie: "anby_session",
			TrustedOrigins: []string{"https://wiki.example.com"},
		}, nil, nil, nil, nil, nil, api)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "blocked.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("must not be stored"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/import-jobs/uploads", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Idempotency-Key", "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4cff")
	request.Header.Set("Origin", "https://evil.example")
	request.AddCookie(&http.Cookie{Name: "anby_session", Value: "attacker-controlled"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if keys := objectStore.Keys(); len(keys) != 0 {
		t.Fatalf("CSRF 拒绝后仍存储对象: %v", keys)
	}
}
