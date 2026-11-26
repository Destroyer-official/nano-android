package frontend

import "embed"

// FS embeds the modular Nano Studio HTML/CSS/JS workstation assets.
//
//go:embed index.html styles.css app.js
var FS embed.FS
