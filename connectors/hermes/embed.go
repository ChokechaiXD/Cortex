package hermesconnector

import "embed"

// ProviderFiles contains the standalone Hermes memory-provider adapter.
//
//go:embed provider/__init__.py provider/client.py provider/extraction.py provider/plugin.yaml provider/README.md
var ProviderFiles embed.FS
