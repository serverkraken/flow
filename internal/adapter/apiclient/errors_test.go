package apiclient

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestIsUnauthorized(t *testing.T) {
	if !IsUnauthorized(&APIError{Method: "GET", Path: "/x", StatusCode: http.StatusUnauthorized}) {
		t.Error("401 APIError should be unauthorized")
	}
	if !IsUnauthorized(fmt.Errorf("wrapped: %w", &APIError{StatusCode: http.StatusUnauthorized})) {
		t.Error("wrapped 401 should be unauthorized")
	}
	if IsUnauthorized(&APIError{StatusCode: http.StatusConflict}) {
		t.Error("409 is not unauthorized")
	}
	if IsUnauthorized(errors.New("network down")) {
		t.Error("plain error is not unauthorized")
	}
	if IsUnauthorized(nil) {
		t.Error("nil is not unauthorized")
	}
}

func TestIsNotFound(t *testing.T) {
	if !IsNotFound(fmt.Errorf("wrapped: %w", &APIError{StatusCode: http.StatusNotFound})) {
		t.Fatal("wrapped 404 should be recognized")
	}
	if IsNotFound(&APIError{StatusCode: http.StatusUnauthorized}) || IsNotFound(nil) {
		t.Fatal("non-404 errors must not be recognized")
	}
}
