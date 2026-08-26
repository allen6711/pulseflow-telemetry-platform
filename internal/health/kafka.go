package health

import (
	"context"
	"fmt"
	"sync"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

// KafkaChecker probes Kafka with a metadata request.
//
// A metadata request, not a TCP dial: a broker with a listening socket that
// cannot serve metadata still accepts connections, and reporting that as ready
// is exactly the failure this probe exists to catch.
type KafkaChecker struct {
	brokers []string

	once    sync.Once
	client  *kgo.Client
	initErr error
}

// NewKafkaChecker returns a checker for the given seed brokers. No connection is
// attempted here; the client is built on first use so an unreachable broker
// cannot prevent the process from starting (FR-037).
func NewKafkaChecker(brokers []string) *KafkaChecker {
	return &KafkaChecker{brokers: brokers}
}

func (c *KafkaChecker) Name() string { return DependencyKafka }

func (c *KafkaChecker) Check(ctx context.Context) error {
	c.once.Do(func() {
		c.client, c.initErr = kgo.NewClient(kgo.SeedBrokers(c.brokers...))
	})
	if c.initErr != nil {
		return fmt.Errorf("building kafka client: %w", c.initErr)
	}

	if _, err := kadm.NewClient(c.client).Metadata(ctx); err != nil {
		return fmt.Errorf("kafka metadata request: %w", err)
	}
	return nil
}

// Close releases the underlying client.
func (c *KafkaChecker) Close() error {
	if c.client != nil {
		c.client.Close()
	}
	return nil
}
