package webhook_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/webhook"
)

func signStripe(secret string, ts int64, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%d.%s", ts, payload)))
	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

func TestVerifyStripeSignature_Valid(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{"id":"evt_1","type":"charge.succeeded"}`)
	header := signStripe(secret, time.Now().Unix(), payload)

	if err := webhook.VerifyStripeSignature(secret, payload, header, 5*time.Minute); err != nil {
		t.Fatalf("expected valid signature to verify, got: %v", err)
	}
}

func TestVerifyStripeSignature_WrongSecret(t *testing.T) {
	payload := []byte(`{"id":"evt_1"}`)
	header := signStripe("whsec_real", time.Now().Unix(), payload)

	if err := webhook.VerifyStripeSignature("whsec_wrong", payload, header, 5*time.Minute); err == nil {
		t.Fatalf("expected signature mismatch with the wrong secret, got nil error")
	}
}

func TestVerifyStripeSignature_TamperedPayload(t *testing.T) {
	secret := "whsec_test"
	header := signStripe(secret, time.Now().Unix(), []byte(`{"id":"evt_1","amount":100}`))

	tampered := []byte(`{"id":"evt_1","amount":999999}`)
	if err := webhook.VerifyStripeSignature(secret, tampered, header, 5*time.Minute); err == nil {
		t.Fatalf("expected a tampered payload to fail verification, got nil error")
	}
}

func TestVerifyStripeSignature_ExpiredOutsideTolerance(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{"id":"evt_1"}`)
	oldTs := time.Now().Add(-1 * time.Hour).Unix()
	header := signStripe(secret, oldTs, payload)

	if err := webhook.VerifyStripeSignature(secret, payload, header, 5*time.Minute); err == nil {
		t.Fatalf("expected a signature older than the tolerance window to be rejected")
	}
}

func TestVerifyStripeSignature_MalformedHeader(t *testing.T) {
	if err := webhook.VerifyStripeSignature("secret", []byte("{}"), "not-a-valid-header", 5*time.Minute); err == nil {
		t.Fatalf("expected a malformed header to fail verification")
	}
}
