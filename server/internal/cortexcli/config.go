package cortexcli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	c2jconfig "github.com/colony-2/c2j/pkg/config"
	"github.com/colony-2/colony2/server/internal/cortex"
)

// Use c2j's loader for discovery, validation and command-valued settings.
// The CLI defaults and URI parser in c2j live under internal/, so only the
// connection adapter and precedence live here.
func resolveConnection(ctx context.Context, cfg cortex.Config, explicit, legacy string) (cortex.Config, error) {
	raw := strings.TrimSpace(explicit)
	legacyTarget := false
	if raw == "" && strings.TrimSpace(legacy) != "" {
		raw, legacyTarget = strings.TrimSpace(legacy), true
	}
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("C2J_JOBDB"))
	}
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("JOBDB_URL"))
		legacyTarget = raw != ""
	}
	if raw == "" {
		project, err := c2jconfig.LoadProjectConfig(cfg.WorkingDir)
		if err != nil && !errors.Is(err, c2jconfig.ErrConfigNotFound) {
			return cfg, err
		}
		if project != nil {
			raw, err = project.JobDBURI(ctx)
			if err != nil {
				return cfg, err
			}
		}
	}
	if raw == "" {
		return cfg, fmt.Errorf("JobDB URI is required: use --jobdb, C2J_JOBDB, or jobdb in .c2j/config.yaml")
	}
	target, err := url.Parse(raw)
	if err != nil {
		return cfg, fmt.Errorf("invalid JobDB URI")
	}
	if (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
		return cfg, fmt.Errorf("Cortex requires a remote JobDB URI: http(s)://host/<tenant-id>")
	}
	if target.User != nil || target.RawQuery != "" || target.ForceQuery || target.Fragment != "" {
		return cfg, fmt.Errorf("JobDB URI must not contain credentials, query parameters, or a fragment")
	}
	tenant := strings.TrimSpace(strings.TrimPrefix(target.Path, "/"))
	if tenant == "" && legacyTarget {
		tenant = strings.TrimSpace(cfg.DefaultTenantID)
		if tenant == "" {
			tenant = "1"
		}
	} else if legacyTarget && cfg.DefaultTenantID != "" {
		return cfg, fmt.Errorf("pass the tenant in the JobDB URI; --tenant-id is only supported with a legacy server-only --jobdb-url or JOBDB_URL")
	}
	if tenant == "" || strings.Contains(tenant, "/") {
		return cfg, fmt.Errorf("JobDB URI requires exactly one tenant path segment: http(s)://host/<tenant-id>")
	}
	cfg.JobDBURL = (&url.URL{Scheme: target.Scheme, Host: target.Host}).String()
	cfg.DefaultTenantID = tenant
	return cfg, nil
}
