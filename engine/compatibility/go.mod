module nano_android/engine/compatibility

go 1.26

require nano_android/engine/apk v0.0.0

replace (
	nano_android/engine/apk => ../apk
	nano_android/engine/security => ../security
)
