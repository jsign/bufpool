package bufpool

type cfg struct {
	maxRetries int
}

// Option modifies the default configuration.
type Option func(*cfg)

var defaultCfg = cfg{maxRetries: 1}

// WithMaxRetries sets the maximum number of retries
// finding a suitable shard from the pool before creating
// a new shard.
func WithMaxRetries(max int) Option {
	return func(cfg *cfg) {
		cfg.maxRetries = max
	}
}
