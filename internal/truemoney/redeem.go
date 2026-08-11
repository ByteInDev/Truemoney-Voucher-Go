package truemoney

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// statusEnvelope is the minimal decode of the TrueMoney answer used to
// decide whether the redeem succeeded (only SUCCESS answers are cached).
type statusEnvelope struct {
	Status struct {
		Code string `json:"code"`
	} `json:"status"`
}

// Redeem redeems a TrueWallet voucher for the given phone number.
// Accepts both voucher codes and full gift.truemoney.com URLs.
//
// A successful answer is cached for ten minutes keyed by (code, mobile):
// a client retry after a timeout replays the real answer instead of
// re-redeeming the voucher (which would return TARGET_USER_REDEEMED).
func (c *Client) Redeem(ctx context.Context, voucher, phoneNumber string) (json.RawMessage, error) {
	code, err := VoucherCode(voucher)
	if err != nil {
		return nil, err
	}
	phoneNumber, err = MobileNumber(phoneNumber)
	if err != nil {
		return nil, err
	}

	key := cacheKey(code, phoneNumber)
	if body, ok := c.cache.get(key); ok {
		return body, nil
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

	raw, err := c.doJSON(req)
	if err != nil {
		return nil, err
	}

	var env statusEnvelope
	if json.Unmarshal(raw, &env) == nil && env.Status.Code == "SUCCESS" {
		c.cache.put(key, raw)
	}
	return raw, nil
}