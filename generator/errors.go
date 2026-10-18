package generator

import "errors"

// Sentinel errors returned (wrapped) during generation. Use errors.Is to match them.
var (
	ErrUnsatisfiable     = errors.New("goppelganger: constraints could not be satisfied")
	ErrUnsupportedLocale = errors.New("goppelganger: unsupported locale")
)
