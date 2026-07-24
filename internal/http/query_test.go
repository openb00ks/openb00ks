package httpapi

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestQueryLimit(t *testing.T) {
	t.Parallel()
	newCtx := func(rawQuery string) *gin.Context {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		target := "/"
		if rawQuery != "" {
			target = "/?" + rawQuery
		}
		c.Request = httptest.NewRequest("GET", target, nil)
		return c
	}
	cases := []struct {
		name     string
		query    string
		def, max int
		want     int
	}{
		{"absent uses default", "", 50, 1000, 50},
		{"valid within range", "limit=25", 50, 1000, 25},
		{"at max is kept", "limit=1000", 50, 1000, 1000},
		{"over max uses default", "limit=2000", 50, 1000, 50},
		{"zero uses default", "limit=0", 50, 1000, 50},
		{"negative uses default", "limit=-5", 50, 1000, 50},
		{"non-numeric uses default", "limit=abc", 50, 1000, 50},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := queryLimit(newCtx(tc.query), tc.def, tc.max); got != tc.want {
				t.Errorf("queryLimit(%q, %d, %d) = %d, want %d", tc.query, tc.def, tc.max, got, tc.want)
			}
		})
	}
}
