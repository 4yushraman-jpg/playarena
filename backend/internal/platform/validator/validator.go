package validator

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	reEmail = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	reUUID  = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

// ErrBodyTooLarge is returned by DecodeJSON when the request body exceeds the
// MaxBytesReader limit set by the BodySizeLimit middleware. Handlers that map
// this error to an HTTP status should use http.StatusRequestEntityTooLarge (413).
var ErrBodyTooLarge = errors.New("request body too large")

// ValidationError carries per-field validation failures.
// The Fields map is keyed by the JSON field name; values are human-readable
// messages suitable for a 400 response body.
type ValidationError struct {
	Fields map[string]string `json:"fields"`
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for k, v := range e.Fields {
		parts = append(parts, k+": "+v)
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

// DecodeJSON decodes the JSON request body into dst and validates the result
// using "validate" struct tags.
//
// Returns:
//   - *ValidationError  when decoding succeeds but field rules fail (→ 400)
//   - another error     when the body is missing or not valid JSON (→ 400)
//   - nil               when everything is valid
func DecodeJSON(r *http.Request, dst any) error {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		// http.MaxBytesReader sets this error type when the body exceeds the limit.
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return ErrBodyTooLarge
		}
		if errors.Is(err, io.EOF) {
			return errors.New("request body is required")
		}
		return fmt.Errorf("request body is not valid JSON: %w", err)
	}
	return validateStruct(dst)
}

// validateStruct reflects over a struct pointer and applies rules declared in
// the "validate" tag on each field.
//
// Supported rules (comma-separated, processed in order):
//
//   - omitempty        skip all subsequent rules when the field value is empty
//   - required         value must be non-empty after trimming whitespace
//   - email            value must match a basic e-mail pattern
//   - uuid             value must be a standard hyphenated UUID
//   - alphanum_under   value must contain only letters, digits, and underscores
//   - min=N            string length must be at least N characters
//   - max=N            string length must be at most N characters
func validateStruct(v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}

	rt := rv.Type()
	fieldErrs := make(map[string]string)

	for i := 0; i < rt.NumField(); i++ {
		sf := rt.Field(i)
		fv := rv.Field(i)

		tag := sf.Tag.Get("validate")
		if tag == "" {
			continue
		}

		// Derive the JSON key as the display name for errors.
		jsonName := strings.SplitN(sf.Tag.Get("json"), ",", 2)[0]
		if jsonName == "" || jsonName == "-" {
			jsonName = sf.Name
		}

		// Nil pointer fields have no value — skip all validation rules for them.
		if fv.Kind() == reflect.Ptr && fv.IsNil() {
			continue
		}

		val := fmt.Sprintf("%v", fv.Interface())
		rules := strings.Split(tag, ",")

		for _, rule := range rules {
			if rule == "omitempty" && strings.TrimSpace(val) == "" {
				break // skip remaining rules for this field
			}
			if msg := applyRule(rule, val); msg != "" {
				fieldErrs[jsonName] = msg
				break // report first failure per field
			}
		}
	}

	if len(fieldErrs) > 0 {
		return &ValidationError{Fields: fieldErrs}
	}
	return nil
}

// applyRule returns an error message string when the rule is violated,
// or an empty string when the value passes.
func applyRule(rule, value string) string {
	switch {
	case rule == "omitempty":
		return ""

	case rule == "required":
		if strings.TrimSpace(value) == "" {
			return "is required"
		}

	case rule == "email":
		if !reEmail.MatchString(value) {
			return "must be a valid email address"
		}

	case rule == "uuid":
		if !reUUID.MatchString(value) {
			return "must be a valid UUID"
		}

	case rule == "alphanum_under":
		for _, c := range value {
			if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' {
				return "must contain only letters, numbers, and underscores"
			}
		}

	case strings.HasPrefix(rule, "min="):
		n, err := strconv.Atoi(strings.TrimPrefix(rule, "min="))
		if err == nil && len([]rune(value)) < n {
			return fmt.Sprintf("must be at least %d characters", n)
		}

	case strings.HasPrefix(rule, "max="):
		n, err := strconv.Atoi(strings.TrimPrefix(rule, "max="))
		if err == nil && len([]rune(value)) > n {
			return fmt.Sprintf("must be at most %d characters", n)
		}
	}

	return ""
}
