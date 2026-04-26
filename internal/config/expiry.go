package config

import "time"

const (
	EXPIRY_POLICY_NONE  = "none"  // no expiry at all
	EXPIRY_POLICY_DECAY = "decay" // score decays over time, expire after constant time

	// when used, will use:
	// decay = exp(-ln(2) * age / half_life)
	DECAY_POLICY_EXPONENTIAL = "exponential"
	DECAY_POLICY_LINEAR      = "linear"
)

type ExpiryConfig struct {
	PrefixMaxAge  time.Duration
	ExpiryPolicy  string
	DecayPolicy   string
	DecayHalfLife time.Duration
}

func NewExpiryConfig() ExpiryConfig {
	return ExpiryConfig{
		PrefixMaxAge:  time.Duration(getenvUint64("PREFIX_MAX_AGE", 1800)) * time.Second,
		ExpiryPolicy:  getenv("EXPIRY_POLICY", EXPIRY_POLICY_DECAY),
		DecayPolicy:   getenv("DECAY_POLICY", DECAY_POLICY_EXPONENTIAL),
		DecayHalfLife: getenvDuration("DECAY_HALF_LIFE", 30*time.Minute),
	}
}
