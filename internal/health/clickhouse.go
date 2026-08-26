package health

import (
	"context"
	"fmt"
	"sync"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// ClickHouseChecker probes ClickHouse over the native protocol.
type ClickHouseChecker struct {
	opts *clickhouse.Options

	once    sync.Once
	conn    driver.Conn
	initErr error
}

// ClickHouseConfig is the subset of configuration this checker needs.
type ClickHouseConfig struct {
	Addr     string
	Database string
	User     string
	Password string
}

// NewClickHouseChecker returns a checker for the given server. clickhouse.Open
// does not dial eagerly, so construction cannot fail because the server is down
// (FR-037).
func NewClickHouseChecker(cfg ClickHouseConfig) *ClickHouseChecker {
	return &ClickHouseChecker{
		opts: &clickhouse.Options{
			Addr: []string{cfg.Addr},
			Auth: clickhouse.Auth{
				Database: cfg.Database,
				Username: cfg.User,
				Password: cfg.Password,
			},
		},
	}
}

func (c *ClickHouseChecker) Name() string { return DependencyClickHouse }

func (c *ClickHouseChecker) Check(ctx context.Context) error {
	c.once.Do(func() {
		c.conn, c.initErr = clickhouse.Open(c.opts)
	})
	if c.initErr != nil {
		return fmt.Errorf("building clickhouse client: %w", c.initErr)
	}

	if err := c.conn.Ping(ctx); err != nil {
		return fmt.Errorf("clickhouse ping: %w", err)
	}
	return nil
}

// Close releases the underlying connection.
func (c *ClickHouseChecker) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
