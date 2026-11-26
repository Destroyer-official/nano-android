module nano_android/engine/profiler

go 1.26

require (
	nano_android/engine/mcp_compiler v0.0.0
	nano_android/engine/security v0.0.0
)

replace (
	nano_android/engine/apk => ../apk
	nano_android/engine/compatibility => ../compatibility
	nano_android/engine/mcp_compiler => ../mcp_compiler
	nano_android/engine/security => ../security
	nano_android/engine/ui => ../ui
)
