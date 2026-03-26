package cache

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
)

// JSONCacheRequest describes a read-through JSON cache operation.
type JSONCacheRequest[T any] struct {
	Context  context.Context
	Path     string
	TTL      time.Duration
	DirPerm  os.FileMode
	FilePerm os.FileMode
	Fetch    func(context.Context) (T, error)
}

func IsFresh(path string, ttl time.Duration) bool {
	if ttl <= 0 {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < ttl
}

func ReadJSON[T any](path string) (T, error) {
	var out T
	data, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

func GetOrRefreshJSON[T any](request JSONCacheRequest[T]) (value T, refreshed bool, err error) {
	if IsFresh(request.Path, request.TTL) {
		value, err = ReadJSON[T](request.Path)
		return value, false, err
	}

	value, err = request.Fetch(request.Context)
	if err != nil {
		return value, false, err
	}
	if err := fs.WriteJSONAtomic(request.Path, value, request.DirPerm, request.FilePerm); err != nil {
		return value, false, err
	}
	return value, true, nil
}
