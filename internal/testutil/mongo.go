//go:build integration

package testutil

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var (
	containerOnce sync.Once
	sharedURI     string
	sharedErr     error
	sharedCleanup func()
)

// StartMongo starts a shared MongoDB container and returns a connected client
// plus a cleanup function. The container is shared across all tests in the same
// process — call cleanup only via t.Cleanup.
func StartMongo(t *testing.T) (*mongo.Client, string) {
	t.Helper()

	containerOnce.Do(func() {
		ctx := context.Background()
		container, err := mongodb.Run(ctx, "mongo:7")
		if err != nil {
			sharedErr = fmt.Errorf("start mongodb container: %w", err)
			return
		}
		uri, err := container.ConnectionString(ctx)
		if err != nil {
			sharedErr = fmt.Errorf("get connection string: %w", err)
			return
		}
		sharedURI = uri
		sharedCleanup = func() {
			if termErr := container.Terminate(ctx); termErr != nil {
				// best-effort cleanup
				fmt.Printf("terminate mongodb container: %v\n", termErr)
			}
		}
	})

	if sharedErr != nil {
		t.Fatalf("mongodb container: %v", sharedErr)
	}

	client, err := mongo.Connect(options.Client().ApplyURI(sharedURI))
	if err != nil {
		t.Fatalf("connect to mongodb: %v", err)
	}

	if err := client.Ping(context.Background(), nil); err != nil {
		t.Fatalf("ping mongodb: %v", err)
	}

	t.Cleanup(func() {
		if disconnErr := client.Disconnect(context.Background()); disconnErr != nil {
			t.Logf("disconnect: %v", disconnErr)
		}
	})

	return client, sharedURI
}

// SharedURI returns the DSN for the shared container. Must call StartMongo first.
func SharedURI(t *testing.T) string {
	t.Helper()
	if sharedURI == "" {
		t.Fatal("SharedURI called before StartMongo")
	}
	return sharedURI
}
