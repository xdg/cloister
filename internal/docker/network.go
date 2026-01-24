package docker

import (
	"errors"
	"strings"
)

// CloisterNetworkName is the standard name for the internal cloister network.
// All cloister containers connect to this network to reach the guardian proxy.
const CloisterNetworkName = "cloister-net"

// ErrNetworkConfigMismatch indicates the network exists but has different settings than requested.
var ErrNetworkConfigMismatch = errors.New("network exists with different configuration")

// NetworkInspectInfo contains network details from docker network inspect.
type NetworkInspectInfo struct {
	Name     string `json:"Name"`
	Driver   string `json:"Driver"`
	Scope    string `json:"Scope"`
	Internal bool   `json:"Internal"`
	IPAM     struct {
		Driver string `json:"Driver"`
		Config []struct {
			Subnet  string `json:"Subnet"`
			Gateway string `json:"Gateway"`
		} `json:"Config"`
	} `json:"IPAM"`
}

// NetworkExists checks if a Docker network with the given name exists.
func NetworkExists(name string) (bool, error) {
	var networks []struct {
		Name string `json:"Name"`
	}

	err := RunJSONLines(&networks, "network", "ls", "--filter", "name=^"+name+"$")
	if err != nil {
		return false, err
	}

	// Filter must match exactly (docker filter is a substring match by default)
	for _, n := range networks {
		if n.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// InspectNetwork retrieves detailed information about a Docker network.
func InspectNetwork(name string) (*NetworkInspectInfo, error) {
	var info NetworkInspectInfo

	// docker network inspect with --format '{{json .}}' returns a single JSON object
	err := RunJSON(&info, "network", "inspect", name)
	if err != nil {
		return nil, err
	}

	// Check if we got an empty result (no name means nothing was returned)
	if info.Name == "" {
		return nil, ErrNoResults
	}

	return &info, nil
}

// EnsureNetwork creates a Docker network if it doesn't exist.
// If the network exists, it verifies the internal flag matches the requested setting.
// Returns nil if the network exists with correct settings or was successfully created.
// Returns ErrNetworkConfigMismatch if the network exists with different internal setting.
func EnsureNetwork(name string, internal bool) error {
	exists, err := NetworkExists(name)
	if err != nil {
		return err
	}

	if exists {
		// Network exists, verify configuration matches
		info, err := InspectNetwork(name)
		if err != nil {
			return err
		}

		if info.Internal != internal {
			return ErrNetworkConfigMismatch
		}

		return nil
	}

	// Network doesn't exist, create it
	args := []string{"network", "create"}
	if internal {
		args = append(args, "--internal")
	}
	args = append(args, name)

	_, err = Run(args...)
	if err != nil {
		// Handle race condition: network might have been created between check and create
		var cmdErr *CommandError
		if errors.As(err, &cmdErr) && strings.Contains(cmdErr.Stderr, "already exists") {
			// Network was created by another process, verify it has correct settings
			info, inspectErr := InspectNetwork(name)
			if inspectErr != nil {
				return inspectErr
			}
			if info.Internal != internal {
				return ErrNetworkConfigMismatch
			}
			return nil
		}
		return err
	}

	return nil
}

// EnsureCloisterNetwork creates the standard cloister network if it doesn't exist.
// The network is created with the --internal flag to prevent external network access.
// This is a convenience wrapper around EnsureNetwork(CloisterNetworkName, true).
func EnsureCloisterNetwork() error {
	return EnsureNetwork(CloisterNetworkName, true)
}
