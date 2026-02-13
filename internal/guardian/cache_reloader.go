package guardian

import (
	"sync"

	"github.com/xdg/cloister/internal/clog"
	"github.com/xdg/cloister/internal/config"
)

// ProjectLister provides the list of active projects for cache reloading.
// Satisfied by TokenRegistry and by test mocks.
type ProjectLister interface {
	List() map[string]TokenInfo
}

// CacheReloader encapsulates the logic for rebuilding allowlist and denylist
// caches from static config and on-disk decision files. It replaces the
// closure-based reload logic previously inlined in internal/cmd/guardian.go.
type CacheReloader struct {
	mu              sync.RWMutex
	staticAllow     []config.AllowEntry
	staticDeny      []config.AllowEntry
	globalDecisions *config.Decisions
	cache           *AllowlistCache
	lister          ProjectLister
}

// NewCacheReloader creates a CacheReloader wired to the given cache and project lister.
// staticAllow and staticDeny are the entries from the global static config file.
// globalDecisions is the initial global decisions loaded from disk at startup.
func NewCacheReloader(
	cache *AllowlistCache,
	lister ProjectLister,
	staticAllow, staticDeny []config.AllowEntry,
	globalDecisions *config.Decisions,
) *CacheReloader {
	if globalDecisions == nil {
		globalDecisions = &config.Decisions{}
	}
	return &CacheReloader{
		staticAllow:     staticAllow,
		staticDeny:      staticDeny,
		globalDecisions: globalDecisions,
		cache:           cache,
		lister:          lister,
	}
}

// GlobalDecisions returns the current global decisions (last loaded from disk).
func (r *CacheReloader) GlobalDecisions() *config.Decisions {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.globalDecisions
}

// SetStaticConfig updates the static allow/deny entries. Call this after
// reloading the global config file (e.g. on SIGHUP) and before calling Reload().
func (r *CacheReloader) SetStaticConfig(allow, deny []config.AllowEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.staticAllow = allow
	r.staticDeny = deny
}

// LoadProjectAllowlist loads and returns the merged allowlist for a project.
// It reads the project's static config and decision files from disk, then
// merges them with the global static allow entries and global decisions.
// Returns nil if there are no project-specific entries (matching the
// AllowlistCache convention where nil means "use global allowlist").
func (r *CacheReloader) LoadProjectAllowlist(projectName string) *Allowlist {
	projectCfg, err := config.LoadProjectConfig(projectName)
	if err != nil {
		clog.Warn("failed to load project config for %s: %v", projectName, err)
		return nil
	}

	projectDecisions, err := config.LoadProjectDecisions(projectName)
	if err != nil {
		clog.Warn("failed to load project decisions for %s: %v", projectName, err)
		projectDecisions = &config.Decisions{}
	}

	hasProjectConfig := len(projectCfg.Proxy.Allow) > 0
	hasProjectDecisions := len(projectDecisions.Proxy.Allow) > 0
	if !hasProjectConfig && !hasProjectDecisions {
		return nil
	}

	// Read static config and global decisions under read lock.
	r.mu.RLock()
	staticAllow := r.staticAllow
	globalDecisionsAllow := r.globalDecisions.Proxy.Allow
	r.mu.RUnlock()

	// Merge: global static + project static + global decisions + project decisions
	merged := config.MergeAllowlists(staticAllow, projectCfg.Proxy.Allow)
	if len(globalDecisionsAllow) > 0 {
		merged = append(merged, globalDecisionsAllow...)
	}
	if hasProjectDecisions {
		merged = append(merged, projectDecisions.Proxy.Allow...)
	}
	allowlist := NewAllowlistFromConfig(merged)
	clog.Info("loaded allowlist for project %s (%d entries)", projectName, len(allowlist.Domains()))
	return allowlist
}

// LoadProjectDenylist loads and returns the merged denylist for a project.
// It reads the project's static config and decision files from disk, then
// merges them. Returns nil if no deny entries exist.
func (r *CacheReloader) LoadProjectDenylist(projectName string) *Allowlist {
	projectCfg, err := config.LoadProjectConfig(projectName)
	if err != nil {
		clog.Warn("failed to load project config (deny) for %s: %v", projectName, err)
		return nil
	}

	projectDecisions, err := config.LoadProjectDecisions(projectName)
	if err != nil {
		clog.Warn("failed to load project decisions (deny) for %s: %v", projectName, err)
		projectDecisions = &config.Decisions{}
	}

	projectDeny := config.MergeDenylists(projectCfg.Proxy.Deny, projectDecisions.Proxy.Deny)
	if len(projectDeny) == 0 {
		return nil
	}
	denylist := NewAllowlistFromConfig(projectDeny)
	clog.Info("loaded denylist for project %s: %d static + %d decisions = %d total",
		projectName, len(projectCfg.Proxy.Deny), len(projectDecisions.Proxy.Deny), len(projectDeny))
	return denylist
}

// Reload reloads global decisions from disk and rebuilds all caches.
// It is safe to call concurrently (guarded by internal mutex).
//
// Steps:
//  1. Load global decisions from disk
//  2. Update globalDecisions and snapshot static entries under write lock
//  3. Rebuild global allowlist and denylist on the cache
//  4. Clear per-project caches (denylists are lazily reloaded)
//  5. Eagerly reload per-project allowlists for registered projects
//
// Note: project denylists are NOT eagerly reloaded — they are lazily loaded
// via AllowlistCache.GetProjectDeny calling the denylist loader.
func (r *CacheReloader) Reload() {
	clog.Debug("CacheReloader: reloading global decisions and caches")

	// 1. Load global decisions from disk
	newGlobalDecisions, err := config.LoadGlobalDecisions()
	if err != nil {
		clog.Warn("failed to reload global decisions: %v", err)
		newGlobalDecisions = &config.Decisions{}
	}

	// 2. Update globalDecisions and snapshot static entries under write lock
	r.mu.Lock()
	r.globalDecisions = newGlobalDecisions
	staticAllow := r.staticAllow
	staticDeny := r.staticDeny
	r.mu.Unlock()

	// 3. Rebuild global allowlist: static + decisions allow.
	// Always copy staticAllow to avoid aliasing the long-lived slice.
	globalAllow := make([]config.AllowEntry, len(staticAllow), len(staticAllow)+len(newGlobalDecisions.Proxy.Allow))
	copy(globalAllow, staticAllow)
	globalAllow = append(globalAllow, newGlobalDecisions.Proxy.Allow...)
	r.cache.SetGlobal(NewAllowlistFromConfig(globalAllow))

	// Rebuild global denylist: MergeDenylists(static, decisions)
	globalDeny := config.MergeDenylists(staticDeny, newGlobalDecisions.Proxy.Deny)
	if len(globalDeny) > 0 {
		r.cache.SetGlobalDeny(NewAllowlistFromConfig(globalDeny))
	} else {
		r.cache.SetGlobalDeny(nil)
	}

	// 4. Clear per-project caches
	r.cache.Clear()

	// 5. Eagerly reload per-project allowlists for registered projects.
	// Skip SetProject when LoadProjectAllowlist returns nil (no project-specific
	// entries). Caching nil would prevent GetProject from falling back to the
	// global allowlist — the lazy loader handles this correctly when the cache
	// entry is absent.
	for _, info := range r.lister.List() {
		if info.ProjectName != "" {
			allowlist := r.LoadProjectAllowlist(info.ProjectName)
			if allowlist != nil {
				r.cache.SetProject(info.ProjectName, allowlist)
			}
		}
	}
}
