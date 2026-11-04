// Package tabbyplugin embeds the tabby-cyberviber plugin, installed by `cyberviber install`.
package tabbyplugin

import "embed"

//go:embed package.json index.js
var Files embed.FS
