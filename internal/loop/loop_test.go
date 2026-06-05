package loop

import (
	"context"
	"powerword/internal/config"
	"testing"
)

func TestRunLoop(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		Verbose: true,
		Model:   "test",
	}

	err := RunLoop(ctx, cfg, "test prompt")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}
