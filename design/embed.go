// Package design embeds the HTML mockups, served by the daemon as the browser simulator.
package design

import _ "embed"

//go:embed index.html
var Index []byte
