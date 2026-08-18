package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr         string
	DataDir      string
	OpsSecret    string
	Window       time.Duration
	IdemTTL      time.Duration
	Workers      int
	PublicBase   string
	LoopbackPath string
}

func Load() (Config, error) {
	c := Config{
		Addr:         env("SPILLWAY_ADDR", ":8080"),
		DataDir:      env("SPILLWAY_DATA_DIR", "./data"),
		OpsSecret:    env("SPILLWAY_OPS_SECRET", "dev-ops-secret"),
		Window:       durSec("SPILLWAY_WINDOW_SEC", 300),
		IdemTTL:      durSec("SPILLWAY_IDEM_TTL_SEC", 86400),
		Workers:      envInt("SPILLWAY_WORKERS", 4),
		PublicBase:   strings.TrimRight(env("SPILLWAY_PUBLIC_BASE", "http://127.0.0.1:8080"), "/"),
		LoopbackPath: "/api/v1/loopback",
	}
	if c.OpsSecret == "" {
		return c, fmt.Errorf("SPILLWAY_OPS_SECRET is empty")
	}
	if c.Workers < 1 {
		c.Workers = 1
	}
	if c.Workers > 32 {
		c.Workers = 32
	}
	if !strings.HasPrefix(c.Addr, ":") && !strings.Contains(c.Addr, ":") {
		c.Addr = ":" + c.Addr
	}
	return c, nil
}

func env(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func durSec(key string, def int) time.Duration {
	n := envInt(key, def)
	if n < 1 {
		n = def
	}
	return time.Duration(n) * time.Second
}
