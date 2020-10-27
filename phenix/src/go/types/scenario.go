package types

import (
	"fmt"

	ifaces "phenix/types/interfaces"
	"phenix/types/version"
	v1 "phenix/types/version/v1"
	v2 "phenix/types/version/v2"

	"github.com/mitchellh/mapstructure"
)

func init() {
	var spec interface{}

	spec = new(v2.ScenarioSpec)
	_ = spec.(ifaces.ScenarioSpec)

	spec = v2.ScenarioApp{}
	_ = spec.(ifaces.ScenarioApp)

	spec = v2.ScenarioAppHost{}
	_ = spec.(ifaces.ScenarioAppHost)
}

func DecodeScenarioFromConfig(c Config) (ifaces.ScenarioSpec, error) {
	var (
		iface         interface{}
		latestVersion = version.StoredVersion[c.Kind]
	)

	if c.APIVersion() != latestVersion {
		version := c.Kind + "/" + latestVersion
		upgrader := GetUpgrader(version)

		if upgrader == nil {
			return nil, fmt.Errorf("no upgrader found for scenario version %s", latestVersion)
		}

		var err error

		iface, err = upgrader.Upgrade(c.APIVersion(), c.Spec, c.Metadata)
		if err != nil {
			return nil, fmt.Errorf("upgrading scenario to %s: %w", latestVersion, err)
		}
	} else {
		var err error

		iface, err = version.GetVersionedSpecForKind(c.Kind, c.APIVersion())
		if err != nil {
			return nil, fmt.Errorf("getting versioned spec for config: %w", err)
		}

		if err := mapstructure.Decode(c.Spec, &iface); err != nil {
			return nil, fmt.Errorf("decoding versioned spec: %w", err)
		}
	}

	spec, ok := iface.(ifaces.ScenarioSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec in config")
	}

	return spec, nil
}

type scenario struct{}

func (scenario) Upgrade(version string, spec map[string]interface{}, md ConfigMetadata) (interface{}, error) {
	// This is a dummy topology upgrader to provide an exmaple of how an upgrader
	// might be coded up. The specs in v0 simply assume that some integer values
	// might be represented as strings when in JSON format.

	if version == "v1" {
		var (
			V1 = new(v1.ScenarioSpec)
			V2 = new(v2.ScenarioSpec)
		)

		if err := mapstructure.WeakDecode(spec, &V1); err != nil {
			return nil, fmt.Errorf("decoding scenario into v1 spec: %w", err)
		}

		for _, exp := range V1.AppsF.ExperimentF {
			app := v2.ScenarioApp{
				NameF:     exp.NameF,
				AssetDirF: exp.AssetDirF,
				MetadataF: exp.MetadataF,
			}

			V2.AppsF = append(V2.AppsF, app)
		}

		for _, host := range V1.AppsF.HostF {
			hosts := make([]v2.ScenarioAppHost, len(host.HostsF))

			for i, h1 := range host.HostsF {
				hosts[i] = v2.ScenarioAppHost{
					HostnameF: h1.HostnameF,
					MetadataF: h1.MetadataF,
				}
			}

			app := v2.ScenarioApp{
				NameF:     host.NameF,
				AssetDirF: host.AssetDirF,
				HostsF:    hosts,
			}

			V2.AppsF = append(V2.AppsF, app)
		}

		return V2, nil
	}

	return nil, fmt.Errorf("unknown version %s to upgrade from", version)
}

func init() {
	RegisterUpgrader("Scenario/v2", new(scenario))
}
