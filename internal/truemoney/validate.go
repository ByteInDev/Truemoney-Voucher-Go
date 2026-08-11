package truemoney

import (
	"errors"
	"net/url"
	"strings"
)

// VoucherCode normalizes a voucher code, accepting either the raw code
// or a full gift.truemoney.com campaign URL.
func VoucherCode(voucher string) (string, error) {
	voucher = strings.TrimSpace(voucher)
	if voucher == "" {
		return "", errors.New("voucher code is required")
	}

	if strings.Contains(voucher, "://") {
		parsed, err := url.Parse(voucher)
		if err != nil {
			return "", errors.New("invalid voucher URL")
		}
		if parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), "gift.truemoney.com") ||
			parsed.Path != "/campaign/" {
			return "", errors.New("invalid voucher URL")
		}
		voucher = parsed.Query().Get("v")
	}

	if len(voucher) > 128 {
		return "", errors.New("invalid voucher code")
	}
	if voucher == "" {
		return "", errors.New("voucher code is required")
	}

	for _, char := range voucher {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != '-' && char != '_' {
			return "", errors.New("invalid voucher code")
		}
	}

	return voucher, nil
}

// MobileNumber validates and normalizes a Thai mobile number
// (10 digits, starting with 0).
func MobileNumber(phoneNumber string) (string, error) {
	phoneNumber = strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(phoneNumber))
	if len(phoneNumber) != 10 || phoneNumber[0] != '0' {
		return "", errors.New("mobile number must contain 10 digits and start with 0")
	}
	for _, char := range phoneNumber {
		if char < '0' || char > '9' {
			return "", errors.New("mobile number must contain 10 digits and start with 0")
		}
	}
	return phoneNumber, nil
}