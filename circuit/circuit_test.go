package retry

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetDataWithCircuitBreaker_OpensAndBlocks(t *testing.T) {
	defaultCircuitBreaker = NewCircuitBreaker()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	for i := 0; i < 3; i++ {
		_, _ = GetDataWithCircuitBreaker(server.URL)
	}

	_, err := GetDataWithCircuitBreaker(server.URL)
	if err == nil {
		t.Fatalf("expected circuit open error")
	}

	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("expected 3 calls before open, got %d", got)
	}
}

func TestGetDataWithCircuitBreaker_HalfOpenSuccessCloses(t *testing.T) {
	defaultCircuitBreaker = NewCircuitBreaker()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n <= 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	for i := 0; i < 3; i++ {
		_, _ = GetDataWithCircuitBreaker(server.URL)
	}

	time.Sleep(openTimeout + 50*time.Millisecond)

	body, err := GetDataWithCircuitBreaker(server.URL)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if body != "ok" {
		t.Fatalf("unexpected body: %q", body)
	}
}
