package truemoney

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Redeem redeems a TrueWallet voucher for the given phone number.
// Accepts both voucher codes and full gift.truemoney.com URLs.
func (c *Client) Redeem(ctx context.Context, voucher, phoneNumber string) (json.RawMessage, error) {
	code, err := VoucherCode(voucher)
	if err != nil {
		return nil, err
	}
	phoneNumber, err = MobileNumber(phoneNumber)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://gift.truemoney.com/campaign/vouchers/%s/redeem", code)
	body, err := json.Marshal(map[string]string{"mobile": phoneNumber})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	setBrowserHeaders(req, "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "https://gift.truemoney.com/campaign/card")

	return c.doJSON(req)
}