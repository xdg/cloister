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
	// Validate proxy settings
	if cfg.Proxy.Listen != "" {
		if err := validateListenAddr(cfg.Proxy.Listen, "proxy.listen"); err != nil {
			return err
		}
	}
	if cfg.Proxy.ApprovalTimeout != "" {
		if err := validateDuration(cfg.Proxy.ApprovalTimeout, "proxy.approval_timeout"); err != nil {
			return err
		}
	}
	if cfg.Proxy.RateLimit < 0 {
		return fmt.Errorf("proxy.rate_limit: must be non-negative, got %d", cfg.Proxy.RateLimit)
	}
	if cfg.Proxy.MaxRequestBytes < 0 {
		return fmt.Errorf("proxy.max_request_bytes: must be non-negative, got %d", cfg.Proxy.MaxRequestBytes)
	}

	// Validate request settings
	if cfg.Request.Listen != "" {
		if err := validateListenAddr(cfg.Request.Listen, "request.listen"); err != nil {
			return err
		}
	}
	if cfg.Request.Timeout != "" {
		if err := validateDuration(cfg.Request.Timeout, "request.timeout"); err != nil {
			return err
		}
	}

	// Validate approval settings
	if cfg.Approval.Listen != "" {
		if err := validateListenAddr(cfg.Approval.Listen, "approval.listen"); err != nil {
			return err
		}
	}
	for i, pattern := range cfg.Approval.AutoApprove {
		if err := validateRegex(pattern.Pattern, fmt.Sprintf("approval.auto_approve[%d].pattern", i)); err != nil {
			return err
		}
	}
	for i, pattern := range cfg.Approval.ManualApprove {
		if err := validateRegex(pattern.Pattern, fmt.Sprintf("approval.manual_approve[%d].pattern", i)); err != nil {
			return err
		}
	}

	// Validate log settings
	if cfg.Log.Level != "" {
		if !validLogLevels[cfg.Log.Level] {
			return fmt.Errorf("log.level: invalid value %q, must be one of: debug, info, warn, error", cfg.Log.Level)
		}
	}

	return nil
}

// ValidateProjectConfig validates a parsed ProjectConfig, checking that all
// fields contain valid values. It validates:
//   - Regex patterns in Commands.AutoApprove compile
//
// Note: Remote URL is not validated as required because empty ProjectConfig
// is valid (defaults will be applied later).
//
// Returns nil if the config is valid, or an error with a clear message
// indicating which field is invalid.
func ValidateProjectConfig(cfg *ProjectConfig) error {
	for i, pattern := range cfg.Commands.AutoApprove {
		if err := validateRegex(pattern.Pattern, fmt.Sprintf("commands.auto_approve[%d].pattern", i)); err != nil {
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
		return fmt.Errorf("%s: invalid regex %q: %v", field, pattern, err)
	}
	return nil
}
