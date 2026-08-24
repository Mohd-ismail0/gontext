package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ConnectionMode indicates whether a dependency is bundled or external.
type ConnectionMode string

const (
	ConnectionBundled  ConnectionMode = "bundled"
	ConnectionExternal ConnectionMode = "external"
)

// Config is runtime configuration loaded from environment variables.
type Config struct {
	Profile      string
	ListenAddr   string
	LogLevel     string
	AuthzModelID string

	Postgres PostgresConfig
	S3       S3Config
	NATS     NATSConfig
	OpenFGA  OpenFGAConfig
	OIDC     OIDCConfig
	MCP      MCPConfig
}

// PostgresConfig holds ledger database settings.
type PostgresConfig struct {
	DSN            string
	ConnectionMode ConnectionMode
}

// S3Config holds evidence object-store settings.
type S3Config struct {
	Endpoint       string
	Region         string
	AccessKeyID    string
	SecretAccessKey string
	PathStyle      bool
	BucketRaw      string
	BucketDerived  string
	BucketQuarantine string
	ConnectionMode ConnectionMode
}

// NATSConfig holds event bus settings.
type NATSConfig struct {
	URL            string
	Domain         string
	Credentials    string
	ConnectionMode ConnectionMode
}

// OpenFGAConfig holds authorization service settings.
type OpenFGAConfig struct {
	APIURL         string
	APIToken       string
	StoreID        string
	ModelID        string
	ConnectionMode ConnectionMode
}

// OIDCConfig holds identity provider settings.
type OIDCConfig struct {
	Issuer         string
	Audience       string
	ClientID       string
	ClientSecret   string
	DiscoveryURL   string
	JWKSURL        string
	ClaimSubject   string
	ClaimEmail     string
	ClaimGroups    string
	ClaimOrg       string
	ClaimScopes    string
	ConnectionMode ConnectionMode
}

// MCPConfig holds OAuth protected-resource metadata for MCP (RFC 9728).
type MCPConfig struct {
	ResourceURL            string
	AuthorizationServers   []string
	ScopesSupported        []string
	ResourceDocumentation  string
}

// Load reads configuration from the process environment.
func Load() (Config, error) {
	cfg := Config{
		Profile:      firstEnv("starter", "PROFILE", "CONTEXT_FABRIC_PROFILE"),
		ListenAddr:   firstEnv(":8080", "LISTEN_ADDR", "CONTEXT_FABRIC_LISTEN_ADDR"),
		LogLevel:     firstEnv("info", "LOG_LEVEL", "CONTEXT_FABRIC_LOG_LEVEL"),
		AuthzModelID: firstEnv("", "AUTHZ_MODEL_ID", "OPENFGA_MODEL_ID"),
		Postgres: PostgresConfig{
			DSN:            env("POSTGRES_DSN", ""),
			ConnectionMode: modeEnv("POSTGRES_CONNECTION_MODE", defaultMode()),
		},
		S3: S3Config{
			Endpoint:         env("S3_ENDPOINT", ""),
			Region:           env("S3_REGION", "us-east-1"),
			AccessKeyID:      env("S3_ACCESS_KEY_ID", ""),
			SecretAccessKey:  env("S3_SECRET_ACCESS_KEY", ""),
			PathStyle:        boolEnv("S3_PATH_STYLE", true),
			BucketRaw:        env("S3_BUCKET_RAW", "context-raw"),
			BucketDerived:    env("S3_BUCKET_DERIVED", "context-derived"),
			BucketQuarantine: env("S3_BUCKET_QUARANTINE", "context-quarantine"),
			ConnectionMode:   modeEnv("S3_CONNECTION_MODE", defaultMode()),
		},
		NATS: NATSConfig{
			URL:            env("NATS_URL", ""),
			Domain:         env("NATS_DOMAIN", ""),
			Credentials:    env("NATS_CREDENTIALS", ""),
			ConnectionMode: modeEnv("NATS_CONNECTION_MODE", defaultMode()),
		},
		OpenFGA: OpenFGAConfig{
			APIURL:         firstEnv("", "OPENFGA_API_URL", "OPENFGA_URL"),
			APIToken:       env("OPENFGA_API_TOKEN", ""),
			StoreID:        env("OPENFGA_STORE_ID", ""),
			ModelID:        firstEnv("", "OPENFGA_MODEL_ID", "AUTHZ_MODEL_ID"),
			ConnectionMode: modeEnv("OPENFGA_CONNECTION_MODE", defaultMode()),
		},
		OIDC: OIDCConfig{
			Issuer:         env("OIDC_ISSUER", ""),
			Audience:       env("OIDC_AUDIENCE", "context-fabric"),
			ClientID:       env("OIDC_CLIENT_ID", ""),
			ClientSecret:   env("OIDC_CLIENT_SECRET", ""),
			DiscoveryURL:   env("OIDC_DISCOVERY_URL", ""),
			JWKSURL:        env("OIDC_JWKS_URL", ""),
			ClaimSubject:   env("OIDC_CLAIM_SUBJECT", "sub"),
			ClaimEmail:     env("OIDC_CLAIM_EMAIL", "email"),
			ClaimGroups:    env("OIDC_CLAIM_GROUPS", "groups"),
			ClaimOrg:       env("OIDC_CLAIM_ORGANIZATION", "org_id"),
			ClaimScopes:    env("OIDC_CLAIM_SCOPES", "scope"),
			ConnectionMode: modeEnv("OIDC_CONNECTION_MODE", defaultMode()),
		},
		MCP: MCPConfig{
			ResourceURL:           env("MCP_RESOURCE_URL", ""),
			AuthorizationServers:  splitCSV(firstEnv("", "MCP_AUTHORIZATION_SERVERS", "OIDC_ISSUER")),
			ScopesSupported:       splitCSV(env("MCP_SCOPES_SUPPORTED", "context:search,context:read,context:request_access")),
			ResourceDocumentation: env("MCP_RESOURCE_DOCUMENTATION", "https://docs.context-fabric.io/mcp"),
		},
	}
	if cfg.AuthzModelID == "" {
		cfg.AuthzModelID = cfg.OpenFGA.ModelID
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks required fields for the selected profile.
func (c Config) Validate() error {
	profile := strings.ToLower(strings.TrimSpace(c.Profile))
	switch profile {
	case "demo", "starter", "xsama", "scaled":
	default:
		return fmt.Errorf("invalid PROFILE %q: want demo|starter|xsama|scaled", c.Profile)
	}
	if strings.TrimSpace(c.ListenAddr) == "" {
		return fmt.Errorf("LISTEN_ADDR is required")
	}

	require := func(name, val string) error {
		if strings.TrimSpace(val) == "" {
			return fmt.Errorf("%s is required for profile %s", name, profile)
		}
		return nil
	}

	// All profiles need a ledger DSN when not purely stubbing; demo may still
	// run with empty optional OIDC discovery if local authn is used.
	if err := require("POSTGRES_DSN", c.Postgres.DSN); err != nil {
		return err
	}
	if err := require("NATS_URL", c.NATS.URL); err != nil {
		return err
	}
	if err := require("S3_ENDPOINT", c.S3.Endpoint); err != nil {
		return err
	}
	if err := require("S3_ACCESS_KEY_ID", c.S3.AccessKeyID); err != nil {
		return err
	}
	if err := require("S3_SECRET_ACCESS_KEY", c.S3.SecretAccessKey); err != nil {
		return err
	}

	switch profile {
	case "demo":
		// Demo may use Local identity + in-memory OpenFGA; store/model pins optional.
		return nil
	case "starter", "xsama", "scaled":
		if err := require("OPENFGA_API_URL", c.OpenFGA.APIURL); err != nil {
			return err
		}
		if err := require("AUTHZ_MODEL_ID / OPENFGA_MODEL_ID", c.AuthzModelID); err != nil {
			return err
		}
		if err := require("OIDC_ISSUER", c.OIDC.Issuer); err != nil {
			return err
		}
		if err := require("OIDC_AUDIENCE", c.OIDC.Audience); err != nil {
			return err
		}
		if c.OIDC.DiscoveryURL == "" && c.OIDC.JWKSURL == "" {
			return fmt.Errorf("OIDC_DISCOVERY_URL or OIDC_JWKS_URL is required for profile %s", profile)
		}
		if profile == "scaled" {
			for _, m := range []struct {
				name string
				mode ConnectionMode
			}{
				{"POSTGRES_CONNECTION_MODE", c.Postgres.ConnectionMode},
				{"S3_CONNECTION_MODE", c.S3.ConnectionMode},
				{"NATS_CONNECTION_MODE", c.NATS.ConnectionMode},
				{"OPENFGA_CONNECTION_MODE", c.OpenFGA.ConnectionMode},
				{"OIDC_CONNECTION_MODE", c.OIDC.ConnectionMode},
			} {
				if m.mode != ConnectionExternal {
					return fmt.Errorf("%s must be external for scaled profile", m.name)
				}
			}
		}
	}
	return nil
}

func defaultMode() ConnectionMode {
	p := strings.ToLower(firstEnv("starter", "PROFILE", "CONTEXT_FABRIC_PROFILE"))
	if p == "scaled" {
		return ConnectionExternal
	}
	return ConnectionBundled
}

func modeEnv(key string, def ConnectionMode) ConnectionMode {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case string(ConnectionBundled), string(ConnectionExternal):
		return ConnectionMode(v)
	default:
		return def
	}
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// firstEnv returns the first non-empty env value among keys, or def.
func firstEnv(def string, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return def
}

func boolEnv(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
