module nano_android

go 1.26

require (
	nano_android/adapters v0.0.0
	nano_android/engine/api v0.0.0
	nano_android/engine/apk v0.0.0
	nano_android/engine/backend v0.0.0
	nano_android/engine/compatibility v0.0.0
	nano_android/engine/intelligence v0.0.0
	nano_android/engine/mcp_compiler v0.0.0
	nano_android/engine/media v0.0.0
	nano_android/engine/plugins v0.0.0
	nano_android/engine/profiler v0.0.0
	nano_android/engine/reporting v0.0.0
	nano_android/engine/security v0.0.0
	nano_android/engine/testing v0.0.0
	nano_android/engine/ui v0.0.0
	nano_android/plugins v0.0.0
)

replace (
	github.com/anthropic/droidforge-mcp/engine/profiler_media => ./engine/profiler_media
	nano_android/adapters => ./adapters
	nano_android/engine/api => ./engine/api
	nano_android/engine/apk => ./engine/apk
	nano_android/engine/backend => ./engine/backend
	nano_android/engine/compatibility => ./engine/compatibility
	nano_android/engine/intelligence => ./engine/intelligence
	nano_android/engine/mcp_compiler => ./engine/mcp_compiler
	nano_android/engine/media => ./engine/media
	nano_android/engine/network => ./engine/network
	nano_android/engine/plugins => ./engine/plugins
	nano_android/engine/profiler => ./engine/profiler
	nano_android/engine/reporting => ./engine/reporting
	nano_android/engine/security => ./engine/security
	nano_android/engine/testing => ./engine/testing
	nano_android/engine/ui => ./engine/ui
	nano_android/plugins => ./plugins
)
