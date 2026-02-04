package config

// setIfZero assigns def to *dst when dst is the type's zero value.
// This keeps defaulting logic concise and reduces copy/paste drift.
func setIfZero[T comparable](dst *T, def T) {
	var zero T
	if *dst == zero {
		*dst = def
	}
}
