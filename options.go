package bufpool

type cfg struct {
	maxRetries int
}

type Option func(*cfg)

var defaultCfg = cfg{maxRetries: 1}

func WithMaxRetries(max int) Option {
	return func(cfg *cfg) {
		cfg.maxRetries = max
	}
}
