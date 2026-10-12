package config

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type Config struct {
	Environment string
	Origin      string
	Address     string
	DatabaseURL string
	LogLevel    string
}

type Lookup func(string) (string, bool)

func Load(lookup Lookup) (Config, error) {
	value := func(name, fallback string) string {
		if v, ok := lookup(name); ok {
			return strings.TrimSpace(v)
		}
		return fallback
	}
	c := Config{
		Environment: value("APP_ENV", ""),
		Origin:      value("APP_ORIGIN", ""),
		DatabaseURL: value("DATABASE_URL", ""),
		LogLevel:    value("LOG_LEVEL", "info"),
	}
	if c.Environment != "development" && c.Environment != "test" && c.Environment != "production" {
		return Config{}, errors.New("APP_ENV must be development, test or production")
	}
	origin, err := url.Parse(c.Origin)
	if err != nil || origin.Host == "" || (origin.Scheme != "http" && origin.Scheme != "https") ||
		origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" ||
		(origin.Path != "" && origin.Path != "/") {
		return Config{}, errors.New("APP_ORIGIN must be an HTTP(S) origin without credentials or path")
	}
	if c.Environment == "production" && origin.Scheme != "https" {
		return Config{}, errors.New("APP_ORIGIN requires HTTPS in production")
	}
	c.Origin = strings.TrimSuffix(c.Origin, "/")
	db, err := url.Parse(c.DatabaseURL)
	if err != nil || (db.Scheme != "postgres" && db.Scheme != "postgresql") || db.Hostname() == "" ||
		db.Path == "" || db.Path == "/" || db.Fragment != "" {
		return Config{}, errors.New("DATABASE_URL must be a PostgreSQL connection URL")
	}
	if c.Environment == "production" && db.User != nil {
		password, _ := db.User.Password()
		if password == "ascend_local_development_only" {
			return Config{}, errors.New("DATABASE_URL must not use the development password in production")
		}
	}
	port, err := strconv.Atoi(value("API_PORT", "8080"))
	if err != nil || port < 1 || port > 65535 {
		return Config{}, errors.New("API_PORT must be between 1 and 65535")
	}
	host := value("API_HOST", "127.0.0.1")
	if net.ParseIP(host) == nil && host != "localhost" {
		return Config{}, errors.New("API_HOST must be an IP address or localhost")
	}
	c.Address = net.JoinHostPort(host, strconv.Itoa(port))
	if c.LogLevel != "debug" && c.LogLevel != "info" && c.LogLevel != "warn" && c.LogLevel != "error" {
		return Config{}, errors.New("LOG_LEVEL must be debug, info, warn or error")
	}
	return c, nil
}
