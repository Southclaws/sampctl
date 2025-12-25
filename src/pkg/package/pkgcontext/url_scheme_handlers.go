package pkgcontext

import (
	"sync"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

type urlSchemeHandler interface {
	Ensure(pcx *PackageContext, meta versioning.DependencyMeta) error
}

type urlSchemeHandlerFunc func(pcx *PackageContext, meta versioning.DependencyMeta) error

func (h urlSchemeHandlerFunc) Ensure(pcx *PackageContext, meta versioning.DependencyMeta) error {
	return h(pcx, meta)
}

var (
	urlSchemeHandlersOnce sync.Once
	urlSchemeHandlers     map[string]urlSchemeHandler
)

func getURLSchemeHandlers() map[string]urlSchemeHandler {
	urlSchemeHandlersOnce.Do(func() {
		urlSchemeHandlers = map[string]urlSchemeHandler{
			"plugin": urlSchemeHandlerFunc(func(pcx *PackageContext, meta versioning.DependencyMeta) error {
				return pcx.ensurePluginDependency(meta)
			}),
			"component": urlSchemeHandlerFunc(func(pcx *PackageContext, meta versioning.DependencyMeta) error {
				return pcx.ensureComponentDependency(meta)
			}),
			"includes": urlSchemeHandlerFunc(func(pcx *PackageContext, meta versioning.DependencyMeta) error {
				return pcx.ensureIncludesDependency(meta)
			}),
			"filterscript": urlSchemeHandlerFunc(func(pcx *PackageContext, meta versioning.DependencyMeta) error {
				return pcx.ensureFilterscriptDependency(meta)
			}),
		}
	})
	return urlSchemeHandlers
}

func ensureURLSchemeWithHandler(pcx *PackageContext, meta versioning.DependencyMeta) error {
	h, ok := getURLSchemeHandlers()[meta.Scheme]
	if !ok {
		return errors.Errorf("unsupported URL scheme: %s", meta.Scheme)
	}
	return h.Ensure(pcx, meta)
}
