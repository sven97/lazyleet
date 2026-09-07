package config

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a time.Duration that marshals to/from a human string like "24h"
// in YAML, instead of yaml.v3's default int64 nanoseconds.
type Duration time.Duration

// D returns the underlying time.Duration.
func (d Duration) D() time.Duration { return time.Duration(d) }

func (d Duration) String() string { return time.Duration(d).String() }

// MarshalYAML renders the duration as a string ("24h0m0s").
func (d Duration) MarshalYAML() (any, error) {
	return time.Duration(d).String(), nil
}

// UnmarshalYAML accepts either a duration string ("24h", "90m") or a bare
// number of seconds.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err == nil && s != "" {
		parsed, perr := time.ParseDuration(s)
		if perr != nil {
			return fmt.Errorf("invalid duration %q: %w", s, perr)
		}
		*d = Duration(parsed)
		return nil
	}
	var secs float64
	if err := value.Decode(&secs); err != nil {
		return fmt.Errorf("duration must be a string or number, got %q", value.Value)
	}
	*d = Duration(time.Duration(secs * float64(time.Second)))
	return nil
}
