package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileDownloadRejectsTraversalPayloads(t *testing.T) {
	srv := newTestServer(t)

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{name: "empty id", path: "/files/", wantStatus: http.StatusBadRequest},
		{name: "dot id", path: "/files/.", wantStatus: http.StatusBadRequest},
		{name: "plain traversal", path: "/files/../../secret.txt", wantStatus: http.StatusBadRequest},
		{name: "cleaned away from files prefix", path: "/files/../secret.txt", wantStatus: http.StatusBadRequest},
		{name: "encoded traversal", path: "/files/%2e%2e%2fsecret.txt", wantStatus: http.StatusBadRequest},
		{name: "windows separator traversal", path: `/files/..\..\secret.txt`, wantStatus: http.StatusNotFound},
		{name: "double slash traversal", path: "/files//../secret.txt", wantStatus: http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()

			srv.ensureTransport().HandleFileDownload(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("unexpected status: got=%d want=%d path=%s", rec.Code, tc.wantStatus, tc.path)
			}
		})
	}
}

func TestFileDownloadTraversalNeverLeaksExternalFileContent(t *testing.T) {
	srv := newTestServer(t)
	marker := "TOP_SECRET_MARKER_987654321"
	externalPath := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(externalPath, []byte(marker), 0o600); err != nil {
		t.Fatalf("write external file failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/files/../../"+filepath.Base(externalPath), nil)
	rec := httptest.NewRecorder()
	srv.ensureTransport().HandleFileDownload(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("unexpected 200 for traversal payload")
	}
	if strings.Contains(rec.Body.String(), marker) {
		t.Fatal("external file content leaked through /files handler")
	}
}

func TestFileDownloadTraversalReturnsUnauthorizedWhenTokenRequired(t *testing.T) {
	t.Setenv("AIM_RPC_TOKEN", "test-token")
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/files/../../secret.txt", nil)
	rec := httptest.NewRecorder()
	srv.ensureTransport().HandleFileDownload(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token when auth is required, got %d", rec.Code)
	}

	reqWithToken := httptest.NewRequest(http.MethodGet, "/files/../../secret.txt", nil)
	reqWithToken.Header.Set("X-AIM-RPC-Token", "test-token")
	recWithToken := httptest.NewRecorder()
	srv.ensureTransport().HandleFileDownload(recWithToken, reqWithToken)
	if recWithToken.Code != http.StatusBadRequest {
		t.Fatalf("expected traversal rejection with valid token, got %d", recWithToken.Code)
	}
}
