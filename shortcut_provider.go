package main

func availableShortcutProviders(plugins []Plugin) []string {
	var available []string
	for _, p := range plugins {
		cfg, err := loadPluginConfig(p.Name())
		if err != nil {
			continue
		}
		if pluginConfigComplete(p.ConfigFields(), cfg) {
			available = append(available, p.Name())
		}
	}
	return available
}

// pluginByName returns the registered plugin with the given name, or nil.
func pluginByName(plugins []Plugin, name string) Plugin {
	for _, p := range plugins {
		if p.Name() == name {
			return p
		}
	}
	return nil
}

// resolveShortcutProvider maps a configured provider name onto a registered
// plugin. A config naming a provider that no longer ships (blackbox, removed
// in favour of openrouter) resolves to the default instead of failing at run
// time. The config file is left alone; the resolved value is persisted the
// next time the user cycles providers with 'p'.
func resolveShortcutProvider(name string, plugins []Plugin) string {
	if pluginByName(plugins, name) != nil {
		return name
	}
	return defaultAIShortcutProvider
}

func cycleShortcutProvider(current string, available []string) string {
	if len(available) == 0 {
		return current
	}
	for i, name := range available {
		if name == current {
			return available[(i+1)%len(available)]
		}
	}
	return available[0]
}
