//go:build integration

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/openb00ks/openb00ks/internal/auth"
	"github.com/openb00ks/openb00ks/internal/db"
	"github.com/openb00ks/openb00ks/internal/queue"
	"github.com/openb00ks/openb00ks/internal/storage"
	"github.com/openb00ks/openb00ks/internal/suggest"
	"github.com/openb00ks/openb00ks/internal/testutil"
)

func TestReceiptRerunEndpoints(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	testutil.RequireTable(t, conn, "entities")
	ctx := context.Background()

	suffix := testutil.UniqueSuffix()
	var tenantID string
	if err := conn.GetContext(ctx, &tenantID, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "tenant-"+suffix); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	var entityID string
	if err := conn.GetContext(ctx, &entityID, `INSERT INTO entities (tenant_id, name) VALUES ($1, $2) RETURNING id`, tenantID, "receipt-int-"+suffix); err != nil {
		t.Fatalf("insert entity: %v", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `DELETE FROM entities WHERE id = $1`, entityID)
		_, _ = conn.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
	}()

	email := "receipt+" + strings.ReplaceAll(suffix, ".", "") + "@test.local"
	hash, _ := auth.HashPassword("testpass123")
	var userID string
	if err := conn.GetContext(ctx, &userID, `INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id`, email, hash); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE users SET default_tenant_id = $2 WHERE id = $1`, userID, tenantID); err != nil {
		t.Fatalf("set default tenant: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO tenant_memberships (tenant_id, user_id, role) VALUES ($1, $2, 'admin')`, tenantID, userID); err != nil {
		t.Fatalf("insert tenant membership: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO entity_users (user_id, entity_id, role) VALUES ($1, $2, 'admin')`, userID, entityID); err != nil {
		t.Fatalf("insert membership: %v", err)
	}

	tokens, _ := auth.NewTokenService("test-secret-32-bytes-exactly----!", time.Now)
	token, _ := tokens.Issue(userID, tenantID, time.Hour)

	objects := storage.NewLocalStore(os.TempDir(), "")
	hc := NewHandlerContext(conn, tokens, time.Hour, 0, nil, suggest.Pricing{}, objects, NewReceiptHandler(10*1024*1024), SystemInfo{})
	server := NewServer(hc)
	hc.SetStores(db.NewStores(conn), queue.NewDBQueue(conn))

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("entity_id", entityID)
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", `form-data; name="file"; filename="test-`+suffix+`.png"`)
	partHeader.Set("Content-Type", "image/png")
	part, _ := writer.CreatePart(partHeader)
	part.Write([]byte("\x89PNG\r\n\x1a\n")) // PNG magic bytes
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/receipts", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	receiptID, _ := resp["id"].(string)
	if receiptID == "" {
		t.Fatal("missing receipt id")
	}

	for _, path := range []string{
		"/receipts/" + receiptID + "/ocr/rerun",
		"/receipts/" + receiptID + "/suggestion/rerun",
		"/receipts/" + receiptID + "/draft/rerun",
	} {
		req = httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w = httptest.NewRecorder()
		server.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", path, w.Code)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/receipts/"+receiptID+"/ocr", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for ocr, got %d", w.Code)
	}
}
