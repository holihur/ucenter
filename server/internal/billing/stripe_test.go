package billing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStripeCheckoutAdapter(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth, _, _ = r.BasicAuth()
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("line_items[0][price_data][unit_amount]") != "2900" {
			t.Errorf("price not sent: %v", r.Form)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cs_test_123","url":"https://checkout.stripe.test/cs_test_123"}`))
	}))
	defer srv.Close()
	p := NewStripe("sk_test_dummy", "https://app.test/ok", "https://app.test/cancel")
	p.BaseURL = srv.URL
	out, err := p.CreateCheckout(context.Background(), "tenant-1", "pro")
	if err != nil {
		t.Fatal(err)
	}
	if out.SessionID != "cs_test_123" || !strings.Contains(out.URL, "checkout.stripe.test") {
		t.Fatalf("unexpected checkout: %+v", out)
	}
	if gotPath != "/v1/checkout/sessions" || gotAuth != "sk_test_dummy" {
		t.Fatalf("path/auth = %s/%s", gotPath, gotAuth)
	}
}
