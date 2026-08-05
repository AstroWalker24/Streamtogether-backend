package validator

import govalidator "github.com/go-playground/validator/v10"

// CustomRule describes a named validation function to be registered.
type CustomRule struct {
	// Tag is the struct tag name used in `validate:"tag"`.
	Tag string
	// Fn is the validation function.
	Fn govalidator.Func
}

// RegisterRules registers a slice of CustomRule entries on the Validator.
// Call this once during application bootstrap before serving requests.
// Returns the first registration error encountered, if any.
func (val *Validator) RegisterRules(rules []CustomRule) error {
	for _, r := range rules {
		if err := val.RegisterCustom(r.Tag, r.Fn); err != nil {
			return err
		}
	}
	return nil
}
