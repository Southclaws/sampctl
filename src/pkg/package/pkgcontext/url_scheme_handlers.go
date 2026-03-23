package pkgcontext

import (
	"context"
	"sync"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

type urlSchemeHandler interface {
	Ensure(ctx context.Context, pcx *PackageContext, meta versioning.DependencyMeta) error
}

type urlSchemeHandlerFunc func(ctx context.Context, pcx *PackageContext, meta versioning.DependencyMeta) error

func (h urlSchemeHandlerFunc) Ensure(ctx context.Context, pcx *PackageContext, meta versioning.DependencyMeta) error {
	return h(ctx, pcx, meta)
}

var (
	urlSchemeHandlersOnce sync.Once
	urlSchemeHandlers     map[string]urlSchemeHandler
)

func getURLSchemeHandlers() map[string]urlSchemeHandler {
	urlSchemeHandlersOnce.Do(func() {
		urlSchemeHandlers = map[string]urlSchemeHandler{
			"plugin": urlSchemeHandlerFunc(func(ctx context.Context, pcx *PackageContext, meta versioning.DependencyMeta) error {
				return pcx.ensurePluginDependency(ctx, meta)
			}),
			"component": urlSchemeHandlerFunc(func(ctx context.Context, pcx *PackageContext, meta versioning.DependencyMeta) error {
				return pcx.ensureComponentDependency(ctx, meta)
			}),
			"includes": urlSchemeHandlerFunc(func(ctx context.Context, pcx *PackageContext, meta versioning.DependencyMeta) error {
				return pcx.ensureIncludesDependency(ctx, meta)
			}),
			"filterscript": urlSchemeHandlerFunc(func(ctx context.Context, pcx *PackageContext, meta versioning.DependencyMeta) error {
				return pcx.ensureFilterscriptDependency(ctx, meta)
			}),
		}
	})
	return urlSchemeHandlers
}

func ensureURLSchemeWithHandler(ctx context.Context, pcx *PackageContext, meta versioning.DependencyMeta) error {
	h, ok := getURLSchemeHandlers()[meta.Scheme]
	if !ok {
		return errors.Errorf("unsupported URL scheme: %s", meta.Scheme)
	}
	return h.Ensure(ctx, pcx, meta)
}
