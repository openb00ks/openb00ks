package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/openb00ks/openb00ks/internal/suggest"
)

type readyErr struct{}

func (readyErr) Ready(context.Context) error {
	return errors.New("down")
}

func TestRequireDBBlocksRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hc := NewHandlerContext(readyErr{}, nil, 0, 0, nil, suggest.Pricing{}, nil, NewReceiptHandler(0), SystemInfo{})
	server := NewServer(hc)

	req := httptest.NewRequest(http.MethodGet, "/entities", nil)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestHealthzBypassesDB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hc := NewHandlerContext(readyErr{}, nil, 0, 0, nil, suggest.Pricing{}, nil, NewReceiptHandler(0), SystemInfo{})
	server := NewServer(hc)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, w.Code)
	}
}
