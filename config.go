package gx

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// ── Framework Environment Variables ─────────────────────────────────────────

// applyEnvConfig reads GX_* environment variables and applies them to the app.
// Called during app initialization. Skipped if IgnoreEnv() option was used.
func (a *App) applyEnvConfig() {
	if a.ignoreEnv {
		return
	}

	// GX_ENV
	if v := os.Getenv("GX_ENV"); v != "" {
		a.environment = Environment(v)
	}

	// GX_LOG_LEVEL and GX_LOG_FORMAT are handled by observability layer

	// GX_MAX_BODY_SIZE
	if v := os.Getenv("GX_MAX_BODY_SIZE"); v != "" {
		if size, err := parseSize(v); err == nil {
			a.maxBodySize = size
		}
	}

	// GX_TRUSTED_PROXIES
	if v := os.Getenv("GX_TRUSTED_PROXIES"); v != "" {
		a.trustedProxies = strings.Split(v, ",")
	}
}

// parseSize parses a size string like "10MB", "32KB", "1024" into bytes.
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	multipliers := map[string]int64{
		"B":  1,
		"KB": 1 << 10,
		"MB": 1 << 20,
		"GB": 1 << 30,
	}
	for suffix, mult := range multipliers {
		if strings.HasSuffix(s, suffix) {
			n, err := strconv.ParseInt(strings.TrimSuffix(s, suffix), 10, 64)
			if err != nil {
				return 0, err
			}
			return n * mult, nil
		}
	}
	return strconv.ParseInt(s, 10, 64)
}

// ── Generic Config Loader ────────────────────────────────────────────────────

// LoadConfig loads application configuration from environment variables using
// struct field tags:
//
//   - `env:"VAR_NAME"` — variable name to read
//
//   - `default:"value"` — fallback when env var is absent
//
//   - `required:"true"` — returns an error if env var is absent
//
//     type AppConfig struct {
//     DatabaseURL string `env:"DATABASE_URL" required:"true"`
//     RedisURL    string `env:"REDIS_URL"    default:"redis://localhost:6379"`
//     Port        int    `env:"PORT"         default:"8080"`
//     }
//
//     cfg, err := gx.LoadConfig[AppConfig]()
func LoadConfig[T any]() (T, error) {
	var cfg T
	err := loadConfigInto(reflect.ValueOf(&cfg).Elem())
	return cfg, err
}

func loadConfigInto(v reflect.Value) error {
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldVal := v.Field(i)

		if !fieldVal.CanSet() {
			continue
		}

		// Recurse into nested structs
		if field.Type.Kind() == reflect.Struct {
			if err := loadConfigInto(fieldVal); err != nil {
				return err
			}
			continue
		}

		envTag := field.Tag.Get("env")
		if envTag == "" {
			continue
		}

		rawValue := os.Getenv(envTag)
		required := field.Tag.Get("required") == "true"
		defaultVal := field.Tag.Get("default")

		if rawValue == "" {
			if required {
				return fmt.Errorf("config: required environment variable %q is not set", envTag)
			}
			if defaultVal == "" {
				continue
			}
			rawValue = defaultVal
		}

		if err := setFieldFromString(fieldVal, rawValue); err != nil {
			return fmt.Errorf("config: cannot parse %q=%q into %s: %w", envTag, rawValue, field.Type, err)
		}
	}

	return nil
}

func setFieldFromString(v reflect.Value, s string) error {
	switch v.Kind() {
	case reflect.String:
		v.SetString(s)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// Handle time.Duration
		if v.Type() == reflect.TypeOf(time.Duration(0)) {
			d, err := time.ParseDuration(s)
			if err != nil {
				return err
			}
			v.SetInt(int64(d))
			return nil
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return err
		}
		v.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return err
		}
		v.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return err
		}
		v.SetFloat(n)
	case reflect.Bool:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return err
		}
		v.SetBool(b)
	case reflect.Slice:
		// []string from comma-separated value
		if v.Type().Elem().Kind() == reflect.String {
			parts := strings.Split(s, ",")
			sl := reflect.MakeSlice(v.Type(), len(parts), len(parts))
			for i, p := range parts {
				sl.Index(i).SetString(strings.TrimSpace(p))
			}
			v.Set(sl)
		}
	}
	return nil
}
