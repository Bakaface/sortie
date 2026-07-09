package config

// This file collects the small generic merge primitives used to implement the
// config layering rule ("workflow-level overrides project-level overrides
// global") without repeating the same zero-value guard by hand at every call
// site. Each helper mirrors an existing guard shape found in loadGlobalConfig
// / loadProjectConfig (config.go) and the workflow/project accessor fallbacks
// (accessors.go) — none of them change behavior, they just name the pattern.

// override sets *dst = src when src is not the zero value for T. This is the
// most common shape: an empty string, a nil pointer, or a nil slice-typed
// pointer field all mean "not set in this layer, inherit whatever cfg already
// has". Works for strings, ints (where 0 genuinely means unset), and
// pointer-typed fields (e.g. *bool option flags) where the guard is "src != nil"
// and the assignment copies the pointer itself.
func override[T comparable](dst *T, src T) {
	var zero T
	if src != zero {
		*dst = src
	}
}

// overridePositive sets *dst = src when src > 0. Distinct from override[int]
// because a couple of int fields (max_workers) treat negative values, not just
// zero, as "not configured" — preserved as its own helper rather than folded
// into override so that guard difference stays visible.
func overridePositive(dst *int, src int) {
	if src > 0 {
		*dst = src
	}
}

// overrideFromPtr sets *dst = *src when src is non-nil. Used where an optional
// pointer field on ProjectConfig/GlobalConfig (e.g. *bool, *VerificationConfig)
// merges into a plain-typed Config field — the pointer's presence (not its
// pointee's zero-value-ness) signals whether the layer configured it.
func overrideFromPtr[T any](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}

// overrideNonEmptySlice sets *dst = src when src is a non-empty slice. Slices
// aren't `comparable`, so this can't be folded into override[T].
func overrideNonEmptySlice[T any](dst *[]T, src []T) {
	if len(src) > 0 {
		*dst = src
	}
}

// emptiable is implemented by config value types that define their own
// "nothing configured" predicate (e.g. WorktreeSyncPathsConfig), letting the
// override/fallback helpers generalize past comparable types.
type emptiable interface {
	IsEmpty() bool
}

// overrideIfNotEmpty sets *dst = src when src.IsEmpty() is false.
func overrideIfNotEmpty[T emptiable](dst *T, src T) {
	if !src.IsEmpty() {
		*dst = src
	}
}

// firstNonZero returns the first argument that is not the zero value for T,
// or the zero value if every argument is zero. Used by accessor fallbacks
// where a workflow-level value wins over a project-level value (e.g.
// GetWorktreeSetupCommand).
func firstNonZero[T comparable](vals ...T) T {
	var zero T
	for _, v := range vals {
		if v != zero {
			return v
		}
	}
	return zero
}

// firstNonEmptySlice is the slice analogue of firstNonZero.
func firstNonEmptySlice[T any](vals ...[]T) []T {
	for _, v := range vals {
		if len(v) > 0 {
			return v
		}
	}
	return nil
}

// firstNonEmptyValue is the emptiable analogue of firstNonZero.
func firstNonEmptyValue[T emptiable](vals ...T) T {
	for _, v := range vals {
		if !v.IsEmpty() {
			return v
		}
	}
	var zero T
	return zero
}
