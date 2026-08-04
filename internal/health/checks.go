package health

import "context"

// Checker is implemented by any dependency that can report its health.
type Checker interface {
	// Name returns the identifier used in health responses.
	Name() string
	// Check performs the health probe and returns nil when healthy.
	Check(ctx context.Context) error
	// Required returns true when this dependency must be healthy for the
	// application to be considered ready to serve traffic.
	Required() bool
}

// checkerFunc adapts a plain function to the Checker interface.
type checkerFunc struct {
	name     string
	fn       func(context.Context) error
	required bool
}

// NewChecker wraps fn as a Checker with the given name and required flag.
func NewChecker(name string, required bool, fn func(context.Context) error) Checker {
	return &checkerFunc{name: name, fn: fn, required: required}
}

func (c *checkerFunc) Name() string                    { return c.name }
func (c *checkerFunc) Check(ctx context.Context) error { return c.fn(ctx) }
func (c *checkerFunc) Required() bool                  { return c.required }
