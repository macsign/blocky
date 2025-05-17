package config

import "github.com/sirupsen/logrus"

type SafeSearchConfig struct {
	ClientGroups  map[string][]string `yaml:"clientGroups"`
	SearchEngines map[string]SearchEngineConfig
}

type SearchEngineConfig struct {
	Domain          string
	SafeSearchCname string
}

func (c *SafeSearchConfig) IsEnabled() bool {
	return true
}

func (c *SafeSearchConfig) LogConfig(logger *logrus.Entry) {
}

func (c *SafeSearchConfig) SetDefaults() {
	// Since the default depends on the enum values, set it dynamically
	// to avoid having to repeat the values in the annotation.
	c.SearchEngines = map[string]SearchEngineConfig{
		"bing": {
			Domain: "bing.com", SafeSearchCname: "enforcesafesearch.bing.com",
		},
		"google": {
			Domain: "google.com", SafeSearchCname: "forcesafesearch.google.com",
		},
		"brave": {
			Domain: "search.brave.com", SafeSearchCname: "enforcesafesearch.brave.com",
		},
	}
}
