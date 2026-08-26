package health

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisChecker probes Redis with PING.
type RedisChecker struct {
	opts *redis.Options

	once   sync.Once
	client *redis.Client
}

// RedisConfig is the subset of configuration this checker needs.
type RedisConfig struct {
	Addr     string
	Password string
}

// NewRedisChecker returns a checker for the given server. go-redis connects
// lazily, so construction cannot fail because the server is down (FR-037).
func NewRedisChecker(cfg RedisConfig) *RedisChecker {
	return &RedisChecker{
		opts: &redis.Options{
			Addr:     cfg.Addr,
			Password: cfg.Password,
			// A health probe should fail fast rather than spend its whole
			// budget retrying: the caller already bounds it with a timeout, and
			// burning that budget on retries makes every probe report the
			// timeout reason instead of the real one.
			MaxRetries:  -1,
			DialTimeout: time.Second,
		},
	}
}

func (c *RedisChecker) Name() string { return DependencyRedis }

func (c *RedisChecker) Check(ctx context.Context) error {
	c.once.Do(func() { c.client = redis.NewClient(c.opts) })

	if err := c.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	return nil
}

// Close releases the underlying client.
func (c *RedisChecker) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}
