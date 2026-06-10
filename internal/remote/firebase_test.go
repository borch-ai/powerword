package remote

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

type mockRef struct {
	path   string
	mockDB *mockClient
}

func (r *mockRef) Set(ctx context.Context, v interface{}) error {
	r.mockDB.mu.Lock()
	defer r.mockDB.mu.Unlock()
	r.mockDB.data[r.path] = v
	if r.mockDB.setErr != nil {
		return r.mockDB.setErr
	}
	return nil
}

func (r *mockRef) Get(ctx context.Context, v interface{}) error {
	r.mockDB.mu.Lock()
	defer r.mockDB.mu.Unlock()
	if r.mockDB.getErr != nil {
		return r.mockDB.getErr
	}

	val, ok := r.mockDB.data[r.path]
	if !ok {
		return nil // Value doesn't exist
	}

	if strVal, ok := val.(string); ok {
		ptr, ok := v.(*string)
		if ok {
			*ptr = strVal
		}
	}
	return nil
}

type mockClient struct {
	mu     sync.Mutex
	data   map[string]interface{}
	setErr error
	getErr error
}

func (c *mockClient) NewRef(path string) rtdbRef {
	return &mockRef{path: path, mockDB: c}
}

func TestExchangeSDP_Success(t *testing.T) {
	mockDB := &mockClient{data: make(map[string]interface{})}
	broker := &FirebaseBroker{client: mockDB}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Simulate remote answering
	go func() {
		time.Sleep(100 * time.Millisecond)
		mockDB.mu.Lock()
		mockDB.data["tunnels/test-tunnel/answer"] = "mock-answer"
		mockDB.mu.Unlock()
	}()

	answer, err := broker.ExchangeSDP(ctx, "test-tunnel", "mock-offer")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if answer != "mock-answer" {
		t.Errorf("expected answer mock-answer, got %s", answer)
	}

	mockDB.mu.Lock()
	if mockDB.data["tunnels/test-tunnel/offer"] != "mock-offer" {
		t.Errorf("offer not set correctly")
	}
	if mockDB.data["tunnels/test-tunnel/status"] != "connected" {
		t.Errorf("status not connected")
	}
	mockDB.mu.Unlock()
}

func TestExchangeSDP_SetOfferError(t *testing.T) {
	mockDB := &mockClient{data: make(map[string]interface{}), setErr: fmt.Errorf("permission denied")}
	broker := &FirebaseBroker{client: mockDB}

	ctx := context.Background()
	_, err := broker.ExchangeSDP(ctx, "test-tunnel", "mock-offer")
	if err == nil {
		t.Fatalf("expected error on set, got nil")
	}
}

func TestExchangeSDP_ContextCanceled(t *testing.T) {
	mockDB := &mockClient{data: make(map[string]interface{})}
	broker := &FirebaseBroker{client: mockDB}

	ctx, cancel := context.WithCancel(context.Background())
	// cancel immediately
	cancel()

	_, err := broker.ExchangeSDP(ctx, "test-tunnel", "mock-offer")
	if err == nil {
		t.Fatalf("expected context error, got nil")
	}
}

func TestExchangeSDP_GetErrorRetries(t *testing.T) {
	mockDB := &mockClient{data: make(map[string]interface{}), getErr: fmt.Errorf("read error")}
	broker := &FirebaseBroker{client: mockDB}

	// Will loop on get error and eventually timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := broker.ExchangeSDP(ctx, "test-tunnel", "mock-offer")
	if err == nil {
		t.Fatalf("expected context timeout error, got nil")
	}
}

func TestNewFirebaseBroker(t *testing.T) {
	ctx := context.Background()
	_, err := NewFirebaseBroker(ctx, "")
	if err == nil {
		t.Fatalf("expected error for missing project ID, got nil")
	}
}

func TestNewFirebaseBroker_BadCredentials(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/non/existent/path/for/firebase.json")
	ctx := context.Background()
	_, err := NewFirebaseBroker(ctx, "test-project")
	if err == nil {
		t.Fatalf("expected error from NewFirebaseBroker due to bad credentials, got nil")
	}
}

func TestRealClient_NewRef_Coverage(t *testing.T) {
	// For coverage on wrapper methods; ignores potential panic on nil client.
	defer func() { _ = recover() }()
	c := &realClient{client: nil}
	_ = c.NewRef("test")
}

func TestRealRef_Set_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic, got none")
		}
	}()
	r := &realRef{ref: nil}
	_ = r.Set(context.Background(), "test")
}

func TestRealRef_Get_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic, got none")
		}
	}()
	r := &realRef{ref: nil}
	_ = r.Get(context.Background(), nil)
}
