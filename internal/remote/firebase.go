package remote

import (
	"context"
	"fmt"
	"log"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/db"
)

// rtdbClient is an abstraction for Firebase RTDB Client to allow mocking
type rtdbClient interface {
	NewRef(path string) rtdbRef
}

// rtdbRef is an abstraction for Firebase RTDB Ref
type rtdbRef interface {
	Set(ctx context.Context, v interface{}) error
	Get(ctx context.Context, v interface{}) error
}

// realClient wraps db.Client
type realClient struct {
	client *db.Client
}

func (c *realClient) NewRef(path string) rtdbRef {
	return &realRef{ref: c.client.NewRef(path)}
}

// realRef wraps db.Ref
type realRef struct {
	ref *db.Ref
}

func (r *realRef) Set(ctx context.Context, v interface{}) error {
	return r.ref.Set(ctx, v)
}

func (r *realRef) Get(ctx context.Context, v interface{}) error {
	return r.ref.Get(ctx, v)
}

// FirebaseBroker handles WebRTC signaling via Firebase Realtime Database
type FirebaseBroker struct {
	client rtdbClient
}

// NewFirebaseBroker initializes a new Firebase broker
func NewFirebaseBroker(ctx context.Context, projectID string) (*FirebaseBroker, error) {
	if projectID == "" {
		return nil, fmt.Errorf("firebase project ID is required")
	}
	conf := &firebase.Config{
		ProjectID:   projectID,
		DatabaseURL: fmt.Sprintf("https://%s-default-rtdb.firebaseio.com/", projectID),
	}

	app, err := firebase.NewApp(ctx, conf)
	if err != nil {
		return nil, fmt.Errorf("failed to init firebase app: %w", err)
	}

	client, err := app.Database(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to init database: %w", err)
	}

	return &FirebaseBroker{client: &realClient{client: client}}, nil
}

// ExchangeSDP writes an offer to the RTDB and waits for the remote answer
func (b *FirebaseBroker) ExchangeSDP(ctx context.Context, tunnelID string, offer string) (string, error) {
	// 1. Write the offer
	refOffer := b.client.NewRef(fmt.Sprintf("tunnels/%s/offer", tunnelID))
	if err := refOffer.Set(ctx, offer); err != nil {
		return "", fmt.Errorf("failed to write offer (are credentials set?): %w", err)
	}

	// Write status
	refStatus := b.client.NewRef(fmt.Sprintf("tunnels/%s/status", tunnelID))
	if err := refStatus.Set(ctx, "pending"); err != nil {
		log.Printf("Warning: failed to set pending status: %v", err)
	}

	log.Printf("Offer written for tunnel %s. Polling for answer...", tunnelID)

	// 2. Poll for the answer
	refAnswer := b.client.NewRef(fmt.Sprintf("tunnels/%s/answer", tunnelID))
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			var answer string
			err := refAnswer.Get(ctx, &answer)
			if err != nil {
				// Database read error (e.g. permission denied)
				log.Printf("Warning: error reading answer from RTDB: %v", err)
				continue
			}
			if answer != "" {
				if err := refStatus.Set(ctx, "connected"); err != nil {
					log.Printf("Warning: failed to set connected status: %v", err)
				}
				return answer, nil
			}
		}
	}
}
