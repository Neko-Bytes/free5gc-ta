package ta

import (
	"os"

	"github.com/free5gc/udr/internal/logger"
	"gopkg.in/yaml.v2"
)

// SeedConfigFromYaml reads udrcfg.yaml dynamically from disk and populates
// UDR's Owner 1 space in Trust Anchor.
//
// NOTE: argument is variadic (...string) so that the function can either be called without arguments "SeedConfigFromYaml()"
// or a custom path from the dev can be provided using "SeedConfigFromYaml(path string)"
func SeedConfigFromYaml(customPath ...string) {
	configPath := "config/udrcfg.yaml"
	// If dev wants to provide another config file
	if len(customPath) > 0 && customPath[0] != "" {
		configPath = customPath[0]
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		logger.InitLog.Warnf("[TA Seed] Could not read configuration file (%s): %v", configPath, err)
		return
	}

	var configMap map[string]interface{}
	if err := yaml.Unmarshal(data, &configMap); err != nil {
		logger.InitLog.Warnf("[TA Seed] Failed to parse YAML (%s): %v", configPath, err)
		return
	}

	// Write the configuration directly to Trust Anchor Owner 1 (Category 0x00)
	if err := TaWrite(1, CategoryDefault, "udr_config", configMap); err != nil {
		logger.InitLog.Warnf("[TA Seed] Failed to write udr_config to Owner 1: %v", err)
	} else {
		logger.InitLog.Infof("[TA Seed] Successfully populated UDR Owner Space (Owner 1) from %s", configPath)
	}
}
