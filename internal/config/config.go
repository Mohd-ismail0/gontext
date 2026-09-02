package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConnectionMode indicates whether a dependency is bundled or external.
type ConnectionMode string

const (
	ConnectionBundled  ConnectionMode = "bundled"
	ConnectionExternal ConnectionMode = "external"
)

// Profile is the deployment packaging profile (ADR 0002).
type Profile string

// IsDemo reports whether the profile is the local/demo packaging profile.
func (p Profile) IsDemo() bool {
	return strings.EqualFold(strings.TrimSpace(string(p)), "demo")
}

// IsProduction reports starter, xsama, or scaled — non-demo profiles that require
// real secrets, OIDC, and fail-closed auth.
func (p Profile) IsProduction() bool {
	switch strings.ToLower(strings.TrimSpace(string(p))) {
	case "starter", "xsama", "scaled":
		return true
	default:
		return false
	}
}

// Config is runtime configuration loaded from environment variables.
type Config struct {
	Profile           Profile
	ListenAddr        string
	LogLevel          string
	AuthzModelID      string
	MetricsListenAddr string

	Secrets Secrets
	Runtime Runtime

	Postgres PostgresConfig
	S3       S3Config
	NATS     NATSConfig
	OpenFGA  OpenFGAConfig
	OIDC     OIDCConfig
	MCP      MCPConfig
}

// Secrets holds classified values loaded from env or *_FILE paths.
type Secrets struct {
	WebhookSigningSecret   string
	DeletionSigningSecret  string
	PlatformBootstrapToken string
	PostgresAdminDSN       string
	GatewayPassword        string
	GatewayPasswordFile    string
}

// Runtime holds process-level toggles and server tuning.
type Runtime struct {
	UseMemory            bool
	AllowStubOps         bool
	AllowScopeHeader     bool
	AllowSkipHMAC        bool
	MetricsListenAddr    string
	ShutdownGraceSeconds int
	IndexBackend         string
	ConfigOverlayPath    string
}

// PostgresConfig holds ledger database settings.
type PostgresConfig struct {
	DSN            string
	ConnectionMode ConnectionMode
}

// S3Config holds evidence object-store settings.
type S3Config struct {
	Endpoint         string
	Region           string
	AccessKeyID      string
	SecretAccessKey  string
	PathStyle        bool
	BucketRaw        string
	BucketDerived    string
	BucketQuarantine string
	ConnectionMode   ConnectionMode
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
	ResourceURL           string
	AuthorizationServers  []string
	ScopesSupported       []string
	ResourceDocumentation string
}

var placeholderValues = map[string]struct{}{
	"change-me":               {},
	"replace-after-bootstrap": {},
	"changeme":                {},
	"todo":                    {},
}

// Load reads configuration from the process environment and validates it.
func Load() (Config, error) {
	cfg, err := LoadFromEnv()
	if err != nil {
		return Config{}, err
	}
	if err := ValidateEscapeHatches(cfg.Profile); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// LoadBase reads configuration and enforces profile escape hatches without
// requiring the full runtime dependency surface (NATS, S3, OIDC, etc.).
// Use for one-shot roles such as migrate and bootstrap.
func LoadBase() (Config, error) {
	cfg, err := LoadFromEnv()
	if err != nil {
		return Config{}, err
	}
	if err := ValidateEscapeHatches(cfg.Profile); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// LoadFromEnv reads configuration from the process environment without validation.
func LoadFromEnv() (Config, error) {
	profile := Profile(firstEnv("starter", "PROFILE", "CONTEXT_FABRIC_PROFILE"))

	webhookSecret, err := secretValue("WEBHOOK_SIGNING_SECRET")
	if err != nil {
		return Config{}, err
	}
	deletionSecret, err := secretValue("DELETION_SIGNING_SECRET")
	if err != nil {
		return Config{}, err
	}
	bootstrapToken, err := secretValue("PLATFORM_BOOTSTRAP_TOKEN")
	if err != nil {
		return Config{}, err
	}
	adminDSN, err := secretValue("POSTGRES_ADMIN_DSN")
	if err != nil {
		return Config{}, err
	}
	gatewayPassword, gatewayPasswordFile, err := gatewayPasswordValue()
	if err != nil {
		return Config{}, err
	}
	postgresDSN, err := secretValue("POSTGRES_DSN")
	if err != nil {
		return Config{}, err
	}
	s3Secret, err := secretValue("S3_SECRET_ACCESS_KEY")
	if err != nil {
		return Config{}, err
	}
	openfgaToken, err := secretValue("OPENFGA_API_TOKEN")
	if err != nil {
		return Config{}, err
	}
	oidcSecret, err := secretValue("OIDC_CLIENT_SECRET")
	if err != nil {
		return Config{}, err
	}
	natsCreds, err := secretValue("NATS_CREDENTIALS")
	if err != nil {
		return Config{}, err
	}
	openfgaStoreID, err := secretValue("OPENFGA_STORE_ID")
	if err != nil {
		return Config{}, err
	}

	overlayPath := strings.TrimSpace(os.Getenv("CONTEXT_FABRIC_CONFIG"))
	if overlayPath != "" {
		if err := validateConfigOverlay(overlayPath); err != nil {
			return Config{}, err
		}
	}

	cfg := Config{
		Profile:      profile,
		ListenAddr:   firstEnv(":8080", "LISTEN_ADDR", "CONTEXT_FABRIC_LISTEN_ADDR"),
		LogLevel:     firstEnv("info", "LOG_LEVEL", "CONTEXT_FABRIC_LOG_LEVEL"),
		AuthzModelID: firstEnv("", "AUTHZ_MODEL_ID", "OPENFGA_MODEL_ID"),
		Secrets: Secrets{
			WebhookSigningSecret:   webhookSecret,
			DeletionSigningSecret:  deletionSecret,
			PlatformBootstrapToken: bootstrapToken,
			PostgresAdminDSN:       adminDSN,
			GatewayPassword:        gatewayPassword,
			GatewayPasswordFile:    gatewayPasswordFile,
		},
		Runtime: Runtime{
			UseMemory:            boolEnv("CONTEXT_FABRIC_MEMORY", false),
			AllowStubOps:         boolEnv("CONTEXT_FABRIC_ALLOW_STUB_OPS", false),
			AllowScopeHeader:     boolEnv("CONTEXT_FABRIC_ALLOW_SCOPE_HEADER", false),
			AllowSkipHMAC:        boolEnv("CONTEXT_FABRIC_ALLOW_SKIP_HMAC", false),
			MetricsListenAddr:    firstEnv("", "METRICS_LISTEN_ADDR", "CONTEXT_FABRIC_METRICS_LISTEN_ADDR"),
			ShutdownGraceSeconds: intEnv("SHUTDOWN_GRACE_SECONDS", 10),
			IndexBackend:         strings.TrimSpace(os.Getenv("INDEX_BACKEND")),
			ConfigOverlayPath:    overlayPath,
		},
		Postgres: PostgresConfig{
			DSN:            postgresDSN,
			ConnectionMode: modeEnv("POSTGRES_CONNECTION_MODE", defaultMode()),
		},
		S3: S3Config{
			Endpoint:         env("S3_ENDPOINT", ""),
			Region:           env("S3_REGION", "us-east-1"),
			AccessKeyID:      env("S3_ACCESS_KEY_ID", ""),
			SecretAccessKey:  s3Secret,
			PathStyle:        boolEnv("S3_PATH_STYLE", true),
			BucketRaw:        env("S3_BUCKET_RAW", "context-raw"),
			BucketDerived:    env("S3_BUCKET_DERIVED", "context-derived"),
			BucketQuarantine: env("S3_BUCKET_QUARANTINE", "context-quarantine"),
			ConnectionMode:   modeEnv("S3_CONNECTION_MODE", defaultMode()),
		},
		NATS: NATSConfig{
			URL:            env("NATS_URL", ""),
			Domain:         env("NATS_DOMAIN", ""),
			Credentials:    natsCreds,
			ConnectionMode: modeEnv("NATS_CONNECTION_MODE", defaultMode()),
		},
		OpenFGA: OpenFGAConfig{
			APIURL:         firstEnv("", "OPENFGA_API_URL", "OPENFGA_URL"),
			APIToken:       openfgaToken,
			StoreID:        openfgaStoreID,
			ModelID:        firstEnv("", "OPENFGA_MODEL_ID", "AUTHZ_MODEL_ID"),
			ConnectionMode: modeEnv("OPENFGA_CONNECTION_MODE", defaultMode()),
		},
		OIDC: OIDCConfig{
			Issuer:         env("OIDC_ISSUER", ""),
			Audience:       env("OIDC_AUDIENCE", "context-fabric"),
			ClientID:       env("OIDC_CLIENT_ID", ""),
			ClientSecret:   oidcSecret,
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

	// Demo without Postgres DSN → in-process memory ledger.
	if !cfg.Runtime.UseMemory && postgresDSN == "" && profile.IsDemo() {
		cfg.Runtime.UseMemory = true
	}

	cfg.MetricsListenAddr = cfg.Runtime.MetricsListenAddr

	return cfg, nil
}

// IsDemo reports whether the active profile is demo.
func (c Config) IsDemo() bool {
	return c.Profile.IsDemo()
}

// UseMemory reports whether the process should use in-memory adapters.
func (c Config) UseMemory() bool { return c.Runtime.UseMemory }

// AllowMemoryAuth reports whether local/demo identity and memory OpenFGA are permitted.
func (c Config) AllowMemoryAuth() bool {
	return c.Runtime.UseMemory || c.Profile.IsDemo()
}

// AllowStubOps reports whether stub ops scripts are permitted.
func (c Config) AllowStubOps() bool {
	return c.Runtime.AllowStubOps || c.AllowMemoryAuth()
}

// ValidateEscapeHatches rejects demo-only escape hatches outside the demo profile.
func ValidateEscapeHatches(profile Profile) error {
	if profile.IsDemo() {
		return nil
	}
	if boolEnv("CONTEXT_FABRIC_MEMORY", false) {
		return fmt.Errorf("CONTEXT_FABRIC_MEMORY is forbidden outside demo profile")
	}
	for _, kv := range os.Environ() {
		key, val, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(key, "CONTEXT_FABRIC_ALLOW_") {
			continue
		}
		val = strings.TrimSpace(val)
		if val == "1" || strings.EqualFold(val, "true") {
			return fmt.Errorf("%s is forbidden outside demo profile", key)
		}
	}
	return nil
}

// Validate checks required fields for the selected profile.
func (c Config) Validate() error {
	profile := strings.ToLower(strings.TrimSpace(string(c.Profile)))
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

	if c.UseMemory() {
		return c.validateSecrets(profile)
	}

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
		return c.validateSecrets(profile)
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
		return c.validateSecrets(profile)
	}
	return c.validateSecrets(profile)
}

func (c Config) validateSecrets(profile string) error {
	if profile == "demo" || c.UseMemory() {
		return nil
	}

	checkPlaceholder := func(name, val string) error {
		if val == "" {
			return nil
		}
		if IsPlaceholderValue(val) {
			return fmt.Errorf("%s must not use placeholder value %q", name, val)
		}
		return nil
	}

	fields := []struct {
		name string
		val  string
	}{
		{"WEBHOOK_SIGNING_SECRET", c.Secrets.WebhookSigningSecret},
		{"DELETION_SIGNING_SECRET", c.Secrets.DeletionSigningSecret},
		{"PLATFORM_BOOTSTRAP_TOKEN", c.Secrets.PlatformBootstrapToken},
		{"POSTGRES_ADMIN_DSN", c.Secrets.PostgresAdminDSN},
		{"POSTGRES_GATEWAY_PASSWORD", c.Secrets.GatewayPassword},
		{"S3_SECRET_ACCESS_KEY", c.S3.SecretAccessKey},
		{"OPENFGA_API_TOKEN", c.OpenFGA.APIToken},
		{"OIDC_CLIENT_SECRET", c.OIDC.ClientSecret},
		{"OPENFGA_STORE_ID", c.OpenFGA.StoreID},
	}
	for _, f := range fields {
		if err := checkPlaceholder(f.name, f.val); err != nil {
			return err
		}
	}

	if profile != "demo" && !c.UseMemory() {
		if err := requireSecret("WEBHOOK_SIGNING_SECRET", c.Secrets.WebhookSigningSecret, profile); err != nil {
			return err
		}
		if err := requireSecret("DELETION_SIGNING_SECRET", c.Secrets.DeletionSigningSecret, profile); err != nil {
			return err
		}
	}
	return nil
}

func requireSecret(name, val, profile string) error {
	if strings.TrimSpace(val) == "" {
		return fmt.Errorf("%s is required for profile %s", name, profile)
	}
	return nil
}

// IsPlaceholderValue reports bootstrap placeholder secrets that must be rotated before production.
func IsPlaceholderValue(val string) bool {
	v := strings.ToLower(strings.TrimSpace(val))
	if v == "" {
		return false
	}
	if v == "replace-after-bootstrap" {
		return true
	}
	if strings.Contains(v, "change-me") {
		return true
	}
	_, ok := placeholderValues[v]
	return ok
}

// envFile returns the value of key, or the trimmed contents of key+"_FILE" when key is unset.
func envFile(key string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	path := strings.TrimSpace(os.Getenv(key + "_FILE"))
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func secretValue(key string) (string, error) {
	if path := strings.TrimSpace(os.Getenv(key + "_FILE")); path != "" {
		b, err := os.ReadFile(path)
		if err == nil {
			return strings.TrimSpace(string(b)), nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("read %s_FILE: %w", key, err)
		}
	}
	return strings.TrimSpace(os.Getenv(key)), nil
}

func gatewayPasswordValue() (value, filePath string, err error) {
	if path := strings.TrimSpace(os.Getenv("POSTGRES_GATEWAY_PASSWORD_FILE")); path != "" {
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", "", fmt.Errorf("read POSTGRES_GATEWAY_PASSWORD_FILE: %w", readErr)
		}
		return strings.TrimSpace(string(b)), path, nil
	}
	return strings.TrimSpace(os.Getenv("POSTGRES_GATEWAY_PASSWORD")), "", nil
}

func validateConfigOverlay(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read CONTEXT_FABRIC_CONFIG: %w", err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse CONTEXT_FABRIC_CONFIG: %w", err)
	}
	// Stub validation: ensure deployment config envelope matches deploy/schema/config.schema.json.
	if got, _ := doc["apiVersion"].(string); got != "context-fabric.io/v1" {
		return fmt.Errorf("CONTEXT_FABRIC_CONFIG apiVersion must be context-fabric.io/v1")
	}
	if got, _ := doc["kind"].(string); got != "DeploymentConfig" {
		return fmt.Errorf("CONTEXT_FABRIC_CONFIG kind must be DeploymentConfig")
	}
	if got, _ := doc["profile"].(string); strings.TrimSpace(got) == "" {
		return fmt.Errorf("CONTEXT_FABRIC_CONFIG profile is required")
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

func intEnv(key string, def int) int {
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
