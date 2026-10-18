package parser

import "errors"

// Sentinel errors returned (wrapped) by Parse. Use errors.Is to match them.
var (
	ErrUnknownType       = errors.New("goppelganger: unknown schema type")
	ErrMissingType       = errors.New("goppelganger: schema has no type")
	ErrInvalidConstraint = errors.New("goppelganger: invalid or contradictory constraint")
	ErrInvalidPattern    = errors.New("goppelganger: invalid pattern")
	ErrInvalidRef        = errors.New("goppelganger: invalid $ref")
	// ErrSizeLimitExceeded guards memory: minItems/minLength above
	// MaxGeneratedSize would force huge outputs.
	ErrSizeLimitExceeded = errors.New("goppelganger: size limit exceeded")
)
