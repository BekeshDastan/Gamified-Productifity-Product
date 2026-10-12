package config

import (
	"strings"
	"testing"
)

func validEnv() map[string]string {
	return map[string]string{
		"APP_ENV": "test", "APP_ORIGIN": "http://localhost:3000/",
		"DATABASE_URL": "postgres://user:secret@localhost:5432/ascend_test",
	}
}

func loadMap(values map[string]string) (Config, error) {
	return Load(func(key string) (string, bool) { v, ok := values[key]; return v, ok })
}

func TestDefaults(t *testing.T) {
	c, err := loadMap(validEnv())
	if err != nil {
		t.Fatal(err)
	}
	if c.Address != "127.0.0.1:8080" || c.Origin != "http://localhost:3000" || c.LogLevel != "info" {
		t.Fatalf("unexpected non-secret defaults: %s %s %s", c.Address, c.Origin, c.LogLevel)
	}
}

func TestRejectInvalidConfigWithoutLeakingValue(t *testing.T) {
	for _, tt := range []struct{ key, value string }{
		{"APP_ENV", ""}, {"APP_ENV", "other"}, {"APP_ORIGIN", ""},
		{"APP_ORIGIN", "http://user:secret@localhost"}, {"APP_ORIGIN", "http://localhost/path"},
		{"DATABASE_URL", ""}, {"DATABASE_URL", "mysql://user:secret@localhost/db"},
		{"DATABASE_URL", "postgres://user:secret@localhost/"},
		{"API_PORT", "0"}, {"API_PORT", "65536"}, {"API_PORT", "secret"},
		{"API_HOST", "https://secret"}, {"LOG_LEVEL", "secret"},
	} {
		t.Run(tt.key+"/"+tt.value, func(t *testing.T) {
			values := validEnv()
			values[tt.key] = tt.value
			_, err := loadMap(values)
			if err == nil {
				t.Fatal("expected rejection")
			}
			if !strings.Contains(err.Error(), tt.key) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("error must name setting and redact value: %s", err)
			}
		})
	}
}

func TestProductionBoundary(t *testing.T) {
	values := validEnv()
	values["APP_ENV"] = "production"
	if _, err := loadMap(values); err == nil {
		t.Fatal("HTTP production origin accepted")
	}
	values["APP_ORIGIN"] = "https://ascend.example"
	values["DATABASE_URL"] = "postgres://ascend:ascend_local_development_only@localhost/db"
	if _, err := loadMap(values); err == nil {
		t.Fatal("development password accepted")
	}
	values["DATABASE_URL"] = "postgres://ascend:production-value@db/db?sslmode=verify-full"
	if _, err := loadMap(values); err != nil {
		t.Fatal(err)
	}
}
