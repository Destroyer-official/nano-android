module nano_android/engine/reporting

go 1.26

require (
	nano_android/engine/apk v0.0.0
	nano_android/engine/compatibility v0.0.0
	nano_android/engine/profiler v0.0.0
	nano_android/engine/security v0.0.0
	nano_android/engine/testing v0.0.0
	nano_android/engine/ui v0.0.0
)

replace (
	nano_android/engine/apk => ../apk
	nano_android/engine/compatibility => ../compatibility
	nano_android/engine/mcp_compiler => ../mcp_compiler
	nano_android/engine/media => ../media
	nano_android/engine/network => ../network
	nano_android/engine/profiler => ../profiler
	nano_android/engine/security => ../security
	nano_android/engine/testing => ../testing
	nano_android/engine/ui => ../ui
)
