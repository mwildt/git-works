package actions

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"
)

// ---------- Konfiguration ----------

type Config struct {
	Defaults map[string]string      `yaml:"defaults"` // globale Variablen, in jedem Request nutzbar
	Requests map[string]RequestSpec `yaml:"requests"`
}

type RequestSpec struct {
	Method  string            `yaml:"method"`            // GET, POST, ...
	URL     string            `yaml:"url"`               // darf Variablen enthalten: {{host}}
	Headers map[string]string `yaml:"headers"`           // Werte dürfen Variablen enthalten
	Body    string            `yaml:"body"`              // darf Variablen enthalten
	Vars    map[string]string `yaml:"vars,omitempty"`    // request-lokale Defaults
	Timeout time.Duration     `yaml:"timeout,omitempty"` // z.B. 5s
}

// ---------- Client ----------

type Client struct {
	http   *http.Client
	config Config
}

func NewHttpActions(configPath string) (*Client, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("config lesen: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("yaml parsen: %w", err)
	}
	return &Client{
		http:   &http.Client{Timeout: 30 * time.Second},
		config: cfg,
	}, nil
}

// replace rendert das Template mit text/template.
// Fehlende Variablen führen zu einem Fehler (missingkey=error).
func replace(tpl string, vars map[string]string) (string, error) {
	t, err := template.New("").Option("missingkey=error").Parse(tpl)
	if err != nil {
		return "", fmt.Errorf("template parsen: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("fehlende oder ungültige Variable: %w", err)
	}
	return buf.String(), nil
}

// mergeVars bildet die Variablen-Hierarchie:
// globale Defaults < request-lokale Vars < Aufruf-Variablen.
func mergeVars(layers ...map[string]string) map[string]string {
	merged := make(map[string]string)
	for _, l := range layers {
		for k, v := range l {
			merged[k] = v
		}
	}
	return merged
}

// Send führt den konfigurierten Request aus. vars überschreiben
// ggf. konfigurierte Defaults (global und request-lokal).
func (c *Client) Send(ctx context.Context, name string, vars map[string]string) (*http.Response, error) {
	spec, ok := c.config.Requests[name]
	if !ok {
		return nil, fmt.Errorf("request %q nicht definiert", name)
	}
	all := mergeVars(c.config.Defaults, spec.Vars, vars)

	url, err := replace(spec.URL, all)
	if err != nil {
		return nil, fmt.Errorf("url: %w", err)
	}
	body, err := replace(spec.Body, all)
	if err != nil {
		return nil, fmt.Errorf("body: %w", err)
	}

	var bodyReader io.Reader
	if body != "" {
		bodyReader = bytes.NewReader([]byte(body))
	}

	req, err := http.NewRequestWithContext(ctx, spec.Method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	for k, tpl := range spec.Headers {
		v, err := replace(tpl, all)
		if err != nil {
			return nil, fmt.Errorf("header %s: %w", k, err)
		}
		slog.Default().Info("set header", k, v)
		req.Header.Set(k, v)
	}
	if body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if spec.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
		req = req.WithContext(ctx)
	}
	return c.http.Do(req)
}
