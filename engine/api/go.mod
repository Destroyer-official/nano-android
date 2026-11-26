module nano_android/engine/api

go 1.26

require (
	nano_android/adapters v0.0.0
	nano_android/engine/apk v0.0.0
	nano_android/engine/backend v0.0.0
	nano_android/engine/compatibility v0.0.0
	nano_android/engine/intelligence v0.0.0
	nano_android/engine/mcp_compiler v0.0.0
	nano_android/engine/media v0.0.0
	nano_android/engine/network v0.0.0
	nano_android/engine/plugins v0.0.0
	nano_android/engine/profiler v0.0.0
	nano_android/engine/reporting v0.0.0
	nano_android/engine/security v0.0.0
	nano_android/engine/testing v0.0.0
	nano_android/engine/ui v0.0.0
	nano_android/plugins v0.0.0
)

replace (
	github.com/anthropic/droidforge-mcp/engine/profiler_media => ../profiler_media
	nano_android/adapters => ../../adapters
	nano_android/engine/apk => ../apk
	nano_android/engine/backend => ../backend
	nano_android/engine/compatibility => ../compatibility
	nano_android/engine/intelligence => ../intelligence
	nano_android/engine/mcp_compiler => ../mcp_compiler
	nano_android/engine/media => ../media
	nano_android/engine/network => ../network
	nano_android/engine/plugins => ../plugins
	nano_android/engine/profiler => ../profiler
	nano_android/engine/reporting => ../reporting
	nano_android/engine/security => ../security
	nano_android/engine/testing => ../testing
	nano_android/engine/ui => ../ui
	nano_android/plugins => ../../plugins
)
