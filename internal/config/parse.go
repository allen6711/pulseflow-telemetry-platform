package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Problem is a single configuration failure. It carries the three things
// FR-016 requires a message to state: which variable, what it received, and
// what would have been acceptable.
type Problem struct {
	Variable string
	Got      string
	Want     string
}

func (p Problem) String() string {
	return fmt.Sprintf("%s: got %q, %s", p.Variable, p.Got, p.Want)
}

// collector reads environment variables and accumulates every failure it sees.
// It never returns early, because reporting one problem at a time turns a
// three-mistake environment into three restart cycles.
type collector struct {
	problems []Problem
}

func (c *collector) add(name, got, want string) {
	c.problems = append(c.problems, Problem{Variable: EnvPrefix + name, Got: got, Want: want})
}

// lookup returns the raw value and whether it was set to a non-empty string.
func lookup(name string) (string, bool) {
	v, ok := os.LookupEnv(EnvPrefix + name)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

func (c *collector) str(name, def string) string {
	if v, ok := lookup(name); ok {
		return v
	}
	return def
}

func (c *collector) intVal(name string, def int) int {
	v, ok := lookup(name)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		c.add(name, v, "must be an integer")
		return def
	}
	return n
}

func (c *collector) duration(name string, def time.Duration) time.Duration {
	v, ok := lookup(name)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		c.add(name, v, `must be a duration such as "30s", "1m", or "500ms"`)
		return def
	}
	return d
}

func (c *collector) list(name string, def []string) []string {
	v, ok := lookup(name)
	if !ok {
		return def
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		c.add(name, v, "must contain at least one entry")
		return def
	}
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
