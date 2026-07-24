package httpapi

import (
	"reflect"
	"testing"
	"time"

	"github.com/openb00ks/openb00ks/internal/models"
)

func TestReceiptJobResponse(t *testing.T) {
	created := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	updated := created.Add(2 * time.Minute)
	job := models.ReceiptJob{
		ID:          "job-1",
		ReceiptID:   "receipt-1",
		Stage:       "ocr",
		Status:      "queued",
		Attempts:    1,
		MaxAttempts: 5,
		LockedBy:    "",
		LastError:   "",
		CreatedAt:   created,
		UpdatedAt:   updated,
	}
	resp := receiptJobResponse(job)
	if resp["id"] != "job-1" {
		t.Fatalf("expected id job-1, got %v", resp["id"])
	}
	if resp["stage"] != "ocr" {
		t.Fatalf("expected stage ocr, got %v", resp["stage"])
	}
	if v := resp["locked_until"]; !isNilValue(v) {
		t.Fatalf("expected nil locked_until, got %v", v)
	}
}

func isNilValue(v interface{}) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
