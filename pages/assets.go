package pages

import "embed"

// Files holds the browser assets used by the local Go server. The Pages build
// reads these same files directly, so both deployments share one source.
//
//go:embed map.html app.css theme.js app.js board-tools.mjs auto-fill.mjs live-connection.mjs live-fill.mjs pixel-reveal.mjs
var Files embed.FS
