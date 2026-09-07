package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// VerifyStripeSignature implements Stripe's documented webhook signature
// scheme: the Stripe-Signature header carries "t=<unix ts>,v1=<hex hmac>",
// and the signed payload is "<ts>.<raw body>" HMAC-SHA256'd with the
// endpoint's webhook signing secret (STRIPE_WEBHOOK_SECRET). tolerance
// rejects an old, captured-and-replayed signature even if it's otherwise
// valid — Stripe's own client libraries default to 5 minutes.
func VerifyStripeSignature(secret string, payload []byte, header string, tolerance time.Duration) error {
	var ts int64
	var v1 string
	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			parsed, err := strconv.ParseInt(kv[1], 10, 64)
			if err != nil {
				return fmt.Errorf("malformed timestamp in Stripe-Signature header: %w", err)
			}
			ts = parsed
		case "v1":
			v1 = kv[1]
		}
	}
	if ts == 0 || v1 == "" {
		return fmt.Errorf("malformed Stripe-Signature header: missing t or v1")
	}
	if tolerance > 0 && time.Since(time.Unix(ts, 0)).Abs() > tolerance {
		return fmt.Errorf("signature timestamp outside %s tolerance window", tolerance)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%d.%s", ts, payload)))
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(v1)) {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}
