package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// ParseGlobalConfig parses YAML data into a GlobalConfig struct.
// It returns an error if the YAML is malformed, contains unknown fields,
// or has type mismatches. Missing optional fields become zero values.
// Empty input returns a zero-value GlobalConfig.
func ParseGlobalConfig(data []byte) (*GlobalConfig, error) {
	var cfg GlobalConfig
	if err := strictUnmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse global config: %w", err)
	}
	return &cfg, nil
}

// ParseProjectConfig parses YAML data into a ProjectConfig struct.
// It returns an error if the YAML is malformed, contains unknown fields,
// or has type mismatches. Missing optional fields become zero values.
// Empty input returns a zero-value ProjectConfig.
func ParseProjectConfig(data []byte) (*ProjectConfig, error) {
	var cfg ProjectConfig
	if err := strictUnmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse project config: %w", err)
	}
	return &cfg, nil
}

// strictUnmarshal unmarshals YAML data into v, rejecting unknown fields.
// This helps catch typos in configuration files early.
// Empty input is treated as valid, leaving v at its zero value.
func strictUnmarshal(data []byte, v any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	err := decoder.Decode(v)
	if errors.Is(err, io.EOF) {
		// Empty input is valid - v remains at zero value
		return nil
	}
	if err != nil {
		return fmt.Errorf("decode YAML: %w", err)
	}
	return nil
}

// MarshalProjectConfig marshals a ProjectConfig struct to YAML.
func MarshalProjectConfig(cfg *ProjectConfig) ([]byte, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal project config: %w", err)
	}
	return data, nil
}

// MarshalGlobalConfig marshals a GlobalConfig struct to YAML.
func MarshalGlobalConfig(cfg *GlobalConfig) ([]byte, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal global config: %w", err)
	}
	return data, nil
}
