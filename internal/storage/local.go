package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/colormechadd/maileroo/internal/config"
)

type LocalStorage struct {
	basePath string
}

func NewLocalStorage(cfg config.LocalStorageConfig) (*LocalStorage, error) {
	if err := os.MkdirAll(cfg.BasePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base storage directory: %w", err)
	}
	return &LocalStorage{basePath: cfg.BasePath}, nil
}

func (l *LocalStorage) fullPath(key string) string {
	return filepath.Join(l.basePath, key)
}

func (l *LocalStorage) Save(ctx context.Context, key string, reader io.Reader) error {
	fullPath := l.fullPath(key)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}

	f, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, reader)
	return err
}

func (l *LocalStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return os.Open(l.fullPath(key))
}

func (l *LocalStorage) Delete(ctx context.Context, key string) error {
	return os.Remove(l.fullPath(key))
}

func (l *LocalStorage) Exists(ctx context.Context, key string) (bool, error) {
	_, err := os.Stat(l.fullPath(key))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
