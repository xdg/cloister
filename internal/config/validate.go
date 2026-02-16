package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// validLogLevels defines the allowed log level values.
var validLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

// validAuthMethods defines the allowed auth_method values for agent configs.
var validAuthMethods = map[string]bool{
	string(AuthMethodToken):  true,
	string(AuthMethodAPIKey): true,
}

// ValidateGlobalConfig validates a parsed GlobalConfig, checking that all
// fields contain valid values. It validates:
//   - Port numbers in Listen fields (1-65535 or ":port" format)
//   - Duration strings are parseable (ApprovalTimeout, Request.Timeout)
//   - Regex patterns in AutoApprove and ManualApprove compile
//   - RateLimit is non-negative
//   - MaxRequestBytes is non-negative
//   - Log.Level is one of: debug, info, warn, error (if non-empty)
//
// Returns nil if the config is valid, or an error with a clear message
// indicating which field is invalid.
func ValidateGlobalConfig(cfg *GlobalConfig) error {
	if err := validateProxyConfig(&cfg.Proxy); err != nil {
		return err
	}
	if err := validateRequestConfig(&cfg.Request); err != nil {
		return err
	}
	if err := validateHostexecConfig(&cfg.Hostexec); err != nil {
		return err
	}
	if cfg.Log.Level != "" && !validLogLevels[cfg.Log.Level] {
		return fmt.Errorf("log.level: invalid value %q, must be one of: debug, info, warn, error", cfg.Log.Level)
	}
	for name, agentCfg := range cfg.Agents {
		if err := ValidateAgentConfig(&agentCfg, fmt.Sprintf("agents.%s", name)); err != nil {
			return err
		}
	}
	return nil
}

// validateProxyConfig validates the proxy section of the global config.
func validateProxyConfig(proxy *ProxyConfig) error {
	if proxy.Listen != "" {
		if err := validateListenAddr(proxy.Listen, "proxy.listen"); err != nil {
			return err
		}
	}
	if proxy.ApprovalTimeout != "" {
		if err := validateDuration(proxy.ApprovalTimeout, "proxy.approval_timeout"); err != nil {
			return err
		}
	}
	if proxy.RateLimit < 0 {
		return fmt.Errorf("proxy.rate_limit: must be non-negative, got %d", proxy.RateLimit)
	}
	if proxy.MaxRequestBytes < 0 {
		return fmt.Errorf("proxy.max_request_bytes: must be non-negative, got %d", proxy.MaxRequestBytes)
	}
	return nil
}

// validateRequestConfig validates the request section of the global config.
func validateRequestConfig(req *RequestConfig) error {
	if req.Listen != "" {
		if err := validateListenAddr(req.Listen, "request.listen"); err != nil {
			return err
		}
	}
	if req.Timeout != "" {
		if err := validateDuration(req.Timeout, "request.timeout"); err != nil {
			return err
		}
	}
	return nil
}

// validateHostexecConfig validates the hostexec section of the global config.
func validateHostexecConfig(hostexec *HostexecConfig) error {
	if hostexec.Listen != "" {
		if err := validateListenAddr(hostexec.Listen, "hostexec.listen"); err != nil {
			return err
		}
	}
	for i, pattern := range hostexec.AutoApprove {
		if err := validateRegex(pattern.Pattern, fmt.Sprintf("hostexec.auto_approve[%d].pattern", i)); err != nil {
			return err
		}
	}
	for i, pattern := range hostexec.ManualApprove {
		if err := validateRegex(pattern.Pattern, fmt.Sprintf("hostexec.manual_approve[%d].pattern", i)); err != nil {
			return err
		}
	}
	return nil
}

// ValidateProjectConfig validates a parsed ProjectConfig, checking that all
// fields contain valid values. It validates:
//   - Regex patterns in Hostexec.AutoApprove compile
//   - Regex patterns in Hostexec.ManualApprove compile
//
// Note: Remote URL is not validated as required because empty ProjectConfig
// is valid (defaults will be applied later).
//
// Returns nil if the config is valid, or an error with a clear message
// indicating which field is invalid.
func ValidateProjectConfig(cfg *ProjectConfig) error {
	for i, pattern := range cfg.Hostexec.AutoApprove {
		if err := validateRegex(pattern.Pattern, fmt.Sprintf("hostexec.auto_approve[%d].pattern", i)); err != nil {
			return err
		}
	}
	for i, pattern := range cfg.Hostexec.ManualApprove {
		if err := validateRegex(pattern.Pattern, fmt.Sprintf("hostexec.manual_approve[%d].pattern", i)); err != nil {
			return err
		}
	}
	return nil
}

// validateListenAddr validates a listen address in the format ":port" or "host:port".
// Port must be in the range 1-65535.
func validateListenAddr(addr, field string) error {
	// Find the port portion (after the last colon)
	colonIdx := strings.LastIndex(addr, ":")
	if colonIdx == -1 {
		return fmt.Errorf("%s: invalid format %q, expected host:port or :port", field, addr)
	}

	portStr := addr[colonIdx+1:]
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("%s: invalid port %q in %q", field, portStr, addr)
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s: invalid port number %d, must be 1-65535", field, port)
	}

	return nil
}

// validateDuration validates that a duration string can be parsed by time.ParseDuration.
func validateDuration(d, field string) error {
	_, err := time.ParseDuration(d)
	if err != nil {
		return fmt.Errorf("%s: invalid duration %q", field, d)
	}
	return nil
}

// validateRegex validates that a pattern compiles as a valid regular expression.
// Empty patterns are considered valid (no-op).
func validateRegex(pattern, field string) error {
	if pattern == "" {
		return nil
	}
	_, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("%s: invalid regex %q: %w", field, pattern, err)
	}
	return nil
}

// ValidateAgentConfig validates an AgentConfig, checking that credential fields
// are consistent with the specified auth_method. It validates:
//   - auth_method is one of: "token", "api_key" (if non-empty)
//   - "token" method requires the token field to be set
//   - "api_key" method requires the api_key field to be set
//
// The fieldPrefix is prepended to error messages (e.g., "agents.claude").
//
// Returns nil if the config is valid, or an error with a clear message
// indicating which field is invalid.
func ValidateAgentConfig(cfg *AgentConfig, fieldPrefix string) error {
	if cfg.AuthMethod == "" {
		// No auth_method set - this is valid but may warrant a warning
		// (warnings are handled separately by ValidateAgentConfigWarnings)
		return nil
	}

	// Validate auth_method is a known value
	if !validAuthMethods[cfg.AuthMethod] {
		return fmt.Errorf("%s.auth_method: invalid value %q, must be one of: token, api_key", fieldPrefix, cfg.AuthMethod)
	}

	// Validate required fields based on auth_method
	switch cfg.AuthMethod {
	case string(AuthMethodToken):
		if cfg.Token == "" {
			return fmt.Errorf("%s.token: required when auth_method is \"token\"", fieldPrefix)
		}
	case string(AuthMethodAPIKey):
		if cfg.APIKey == "" {
			return fmt.Errorf("%s.api_key: required when auth_method is \"api_key\"", fieldPrefix)
		}
	}

	return nil
}

// ValidateAgentConfigWarnings returns warnings for an AgentConfig.
// Currently warns if no authentication is configured (auth_method not set
// and no host environment variables would provide credentials).
//
// The fieldPrefix is prepended to warning messages (e.g., "agents.claude").
// The hostEnvVars parameter lists environment variable names present on the host
// that could provide credentials (e.g., "ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN").
//
// Returns a slice of warning messages (empty if no warnings).
func ValidateAgentConfigWarnings(cfg *AgentConfig, fieldPrefix string, hostEnvVars []string) []string {
	var warnings []string

	// Warn if no auth configured
	if cfg.AuthMethod == "" {
		// Check if any host env vars would provide credentials
		hasHostEnvAuth := false
		for _, envVar := range hostEnvVars {
			if envVar != "" {
				hasHostEnvAuth = true
				break
			}
		}
		if !hasHostEnvAuth {
			warnings = append(warnings, fmt.Sprintf("%s: no authentication configured (auth_method not set)", fieldPrefix))
		}
	}

	return warnings
}
