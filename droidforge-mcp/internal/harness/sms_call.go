package harness

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

var (
	// Android 17 OTP pattern detection regexes (multi-lingual and formatted code support)
	reOTPKeywords = regexp.MustCompile(`(?i)\b(otp|verification|verify|code|one-time|pin|password|c[oó]digo|mot de passe|best[aä]tigungscode|auth|passcode|token|secret)\b`)
	reDigitsCode  = regexp.MustCompile(`\b\d{4,8}\b|\b\d{3}[-\s]\d{3}\b`)
)

// DetectAndroid17OTP inspects an SMS body for Android 17 OTP verification code patterns.
// Android 17 introduces a 3-hour delay on READ_SMS for untrusted apps when OTP patterns
// are detected, making automated detection critical for testing scam alert flows.
func DetectAndroid17OTP(body string) bool {
	if body == "" {
		return false
	}
	clean := StripZeroWidthAndBidi(body)
	hasKeyword := reOTPKeywords.MatchString(clean)
	hasDigits := reDigitsCode.MatchString(clean)
	return hasKeyword && hasDigits
}

// BuildSMSConsoleCmd builds the emulator telnet console command for SMS injection.
// Sanitizes the body to prevent telnet command injection via carriage returns or newlines.
func BuildSMSConsoleCmd(sender, body string) string {
	safeSender := strings.TrimSpace(sender)
	safeBody := SanitizeTelnetParam(body)
	return fmt.Sprintf("sms send %s %s", safeSender, safeBody)
}

// BuildCallConsoleCmd builds the emulator telnet console command for GSM call operations.
func BuildCallConsoleCmd(action, number string) (string, error) {
	if !ValidatePhoneNumber(number) {
		return "", fmt.Errorf("invalid phone number %q: contains illegal characters", number)
	}
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" || action == "call" {
		return fmt.Sprintf("gsm call %s", number), nil
	}
	switch action {
	case "accept":
		return fmt.Sprintf("gsm accept %s", number), nil
	case "cancel":
		return fmt.Sprintf("gsm cancel %s", number), nil
	case "hold":
		return fmt.Sprintf("gsm hold %s", number), nil
	default:
		return "", fmt.Errorf("unsupported call action %q (must be call, accept, cancel, or hold)", action)
	}
}

// isEmulatorSerial checks whether the device serial indicates an emulator.
func isEmulatorSerial(serial string) bool {
	s := strings.ToLower(strings.TrimSpace(serial))
	return strings.HasPrefix(s, "emulator-") || strings.HasPrefix(s, "127.0.0.1:") || strings.HasPrefix(s, "localhost:")
}

// InjectSMSOrCall handles SMS and Call injection on an Android emulator via console or ADB emu.
func InjectSMSOrCall(ctx context.Context, dm types.DeviceManager, serial, kind, sender, body string) (*types.SMSInjectResult, error) {
	if dm == nil {
		return nil, fmt.Errorf("device manager is required")
	}

	if !isEmulatorSerial(serial) {
		return nil, fmt.Errorf("sim_inject_sms_or_call is only supported on Android emulators (serial must start with emulator- or 127.0.0.1:)")
	}

	kind = strings.ToLower(strings.TrimSpace(kind))
	sender = strings.TrimSpace(sender)
	if sender == "" {
		return nil, fmt.Errorf("sender number is required")
	}
	if !ValidatePhoneNumber(sender) {
		return nil, fmt.Errorf("invalid sender phone number %q", sender)
	}

	otpDetected := false

	switch kind {
	case "sms":
		otpDetected = DetectAndroid17OTP(body)
		consoleCmd := BuildSMSConsoleCmd(sender, body)

		// Try direct console port first
		_, err := dm.EmulatorConsole(serial, consoleCmd)
		if err != nil {
			// Fallback to adb emu sms send
			safeBody := SanitizeTelnetParam(body)
			_, adbErr := dm.ExecADB(serial, "emu", "sms", "send", sender, safeBody)
			if adbErr != nil {
				return nil, fmt.Errorf("failed to inject SMS via console (%v) and adb emu (%w)", err, adbErr)
			}
		}

	case "call":
		action := strings.ToLower(strings.TrimSpace(body))
		if action == "" {
			action = "call"
		}
		consoleCmd, err := BuildCallConsoleCmd(action, sender)
		if err != nil {
			return nil, err
		}

		_, err = dm.EmulatorConsole(serial, consoleCmd)
		if err != nil {
			// Fallback to adb emu gsm
			_, adbErr := dm.ExecADB(serial, "emu", "gsm", action, sender)
			if adbErr != nil {
				return nil, fmt.Errorf("failed to inject call via console (%v) and adb emu (%w)", err, adbErr)
			}
		}

	default:
		return nil, fmt.Errorf("invalid kind %q: must be 'sms' or 'call'", kind)
	}

	return &types.SMSInjectResult{
		Injected:             true,
		TimestampMS:          time.Now().UnixMilli(),
		Android17OTPDetected: otpDetected,
	}, nil
}
