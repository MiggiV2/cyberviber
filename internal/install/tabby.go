package install

import (
	"os"
	"path/filepath"

	tabbyplugin "github.com/d135-1r43/cyberviber/tabby-plugin"
)

const tabbyPluginName = "tabby-cyberviber"

// TabbyPluginsDir is Tabby's user plugin directory on macOS.
func TabbyPluginsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "tabby", "plugins")
}

// InstallTabbyPlugin writes the embedded plugin to <pluginsDir>/node_modules/tabby-cyberviber.
func InstallTabbyPlugin(pluginsDir string) (string, error) {
	dst := filepath.Join(pluginsDir, "node_modules", tabbyPluginName)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", err
	}
	for _, name := range []string{"package.json", "index.js"} {
		b, err := tabbyplugin.Files.ReadFile(name)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dst, name), b, 0o644); err != nil {
			return "", err
		}
	}
	return dst, nil
}

func RemoveTabbyPlugin(pluginsDir string) error {
	return os.RemoveAll(filepath.Join(pluginsDir, "node_modules", tabbyPluginName))
}
