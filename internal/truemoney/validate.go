package truemoney

import (
	"fmt"
	"net/url"
	"strings"
)

// ValidationError marks an input-validation failure (bad voucher code or
// mobile number) so handlers can answer HTTP 400 instead of 500.
// Parity with the FastAPI and NestJS ports of the same API.
type ValidationError struct {
	msg string
}

func (e *ValidationError) Error() string { return e.msg }

func validationErrorf(format string, args ...any) error {
	return &ValidationError{msg: fmt.Sprintf(format, args...)}
}

// VoucherCode normalizes a voucher code, accepting either the raw code
// or a full gift.truemoney.com campaign URL.
func VoucherCode(voucher string) (string, error) {
	voucher = strings.TrimSpace(voucher)
	if voucher == "" {
		return "", validationErrorf("voucher code is required")
	}

	if strings.Contains(voucher, "://") {
		parsed, err := url.Parse(voucher)
		if err != nil {
			return "", validationErrorf("invalid voucher URL")
		}
		if parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), "gift.truemoney.com") ||
			parsed.Path != "/campaign/" {
			return "", validationErrorf("invalid voucher URL")
		}
		voucher = parsed.Query().Get("v")
	}

	if len(voucher) > 128 {
		return "", validationErrorf("invalid voucher code")
	}
	if voucher == "" {
		return "", validationErrorf("voucher code is required")
	}

	for _, char := range voucher {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != '-' && char != '_' {
			return "", validationErrorf("invalid voucher code")
		}
	}

	return voucher, nil
}

// mobileReplacer strips spaces and dashes from phone input; hoisted so
// the replacement tables are built once instead of per request.
var mobileReplacer = strings.NewReplacer(" ", "", "-", "")

// MobileNumber validates and normalizes a Thai mobile number
// (10 digits, starting with 0).
func MobileNumber(phoneNumber string) (string, error) {
	phoneNumber = mobileReplacer.Replace(strings.TrimSpace(phoneNumber))
	if len(phoneNumber) != 10 || phoneNumber[0] != '0' {
		return "", validationErrorf("mobile number must contain 10 digits and start with 0")
	}
	for _, char := range phoneNumber {
		if char < '0' || char > '9' {
			return "", validationErrorf("mobile number must contain 10 digits and start with 0")
		}
	}
	return phoneNumber, nil
}