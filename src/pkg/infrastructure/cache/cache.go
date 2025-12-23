package cache

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
)

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

func GetOrRefreshJSON[T any](
	ctx context.Context,
	path string,
	ttl time.Duration,
	dirPerm, filePerm os.FileMode,
	fetch func(context.Context) (T, error),
) (value T, refreshed bool, err error) {
	if IsFresh(path, ttl) {
		value, err = ReadJSON[T](path)
		return value, false, err
	}

	value, err = fetch(ctx)
	if err != nil {
		return value, false, err
	}
	if err := fs.WriteJSONAtomic(path, value, dirPerm, filePerm); err != nil {
		return value, false, err
	}
	return value, true, nil
}
