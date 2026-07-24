package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestFormSuggestionContext(t *testing.T) {
	if got := formSuggestionContext("  hello ", "legacy"); got != "hello" {
		t.Fatalf("expected trimmed ctx, got %q", got)
	}
	if got := formSuggestionContext(" ", " legacy "); got != "legacy" {
		t.Fatalf("expected legacy fallback, got %q", got)
	}
	if got := formSuggestionContext("", ""); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestIsAllowedImportType(t *testing.T) {
	cases := []struct {
		ct   string
		want bool
	}{
		{"image/jpeg", true},
		{"image/png", true},
		{"application/pdf", true},
		{"text/csv", true},
		{"text/plain", true},
		{"application/json", false},
	}
	for _, c := range cases {
		if got := isAllowedImportType(c.ct); got != c.want {
			t.Fatalf("content-type %s expected %v", c.ct, c.want)
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	if got := sanitizeFilename("../my file.pdf"); got != "my_file.pdf" {
		t.Fatalf("expected sanitized filename, got %q", got)
	}
}

func TestParseTags(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := strings.NewReader("tags=a,b&tags[]=c&tags[]=d")
	c.Request = httptest.NewRequest("POST", "/", body)
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_ = c.Request.ParseForm()

	values := parseTags(c)
	if len(values) != 5 {
		t.Fatalf("expected 5 tags, got %d", len(values))
	}
}
