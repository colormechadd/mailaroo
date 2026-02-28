package storage

import (
	"context"
	"io"
)

// Storage defines the interface for storing and retrieving email blobs.
type Storage interface {
	Save(ctx context.Context, key string, reader io.Reader) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}
