package generator

import (
	"slices"
	"strings"
	"testing"
)

func TestRenderPluginSourcesKeepsTLSOptionsScoped(t *testing.T) {
	cfg, err := Parse(strings.NewReader(configWithPluginSourceGlobals(`
		"plugin_sources": {
			"official": {"url":"https://official.example.invalid/index.json"},
			"sbmc": {"url":"https://10.23.45.67:8443/index.json","insecure":true}
		},
		"settings": {"plugin.source.legacy":"https://legacy.example.invalid/index.json"}
	`)))
	if err != nil {
		t.Fatal(err)
	}
	script, err := Render(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, settings, ok := strings.Cut(string(script), "config_settings() {\n")
	if !ok {
		t.Fatal("missing config_settings function")
	}
	settings, _, ok = strings.Cut(settings, "\n}\n")
	if !ok {
		t.Fatal("missing end of config_settings function")
	}
	var commands []string
	for _, line := range strings.Split(settings, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "apply_plugin_source ") || strings.HasPrefix(line, "apply_setting 'plugin.source.") {
			commands = append(commands, line)
		}
	}
	want := []string{
		"apply_plugin_source 'official' 'https://official.example.invalid/index.json'",
		"apply_plugin_source 'sbmc' 'https://10.23.45.67:8443/index.json' --insecure",
		"apply_setting 'plugin.source.legacy' 'https://legacy.example.invalid/index.json'",
	}
	slices.Sort(commands)
	if !slices.Equal(commands, want) {
		t.Fatalf("generated plugin source commands:\n%q\nwant:\n%q", commands, want)
	}
}

func TestParseRejectsInvalidPluginSources(t *testing.T) {
	cases := []struct {
		name, globals string
	}{
		{
			name:    "invalid source name",
			globals: `"plugin_sources":{"bad name":{"url":"https://example.invalid/index.json"}}`,
		},
		{
			name: "source configured twice",
			globals: `"plugin_sources":{"mirror":{"url":"https://example.invalid/index.json"}},
				"settings":{"plugin.source.mirror":"https://example.invalid/index.json"}`,
		},
		{
			name:    "insecure public address",
			globals: `"plugin_sources":{"mirror":{"url":"https://8.8.8.8/index.json","insecure":true}}`,
		},
		{
			name:    "insecure hostname",
			globals: `"plugin_sources":{"mirror":{"url":"https://example.invalid/index.json","insecure":true}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(configWithPluginSourceGlobals(tc.globals))); err == nil {
				t.Fatal("accepted invalid plugin source configuration")
			}
		})
	}
}

func configWithPluginSourceGlobals(globals string) string {
	return `{"schema":1,"framework":{"archive":"payload/framework.zip","version":"3.0.0-dev.9"},"globals":{` + globals + `}}`
}
