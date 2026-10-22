package tools

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestAllowlistAndSSRF(t *testing.T) {
	k := New()
	for raw, want := range map[string]bool{
		"https://sjc.com.vn/x": true, "https://www.sjc.com.vn/x": true, "http://sjc.com.vn/": false,
		"https://evilsjc.com.vn/": false, "https://sjc.com.vn:8443/": false, "https://user@sjc.com.vn/": false,
		"https://127.0.0.1/": false, "https://metadata.google.internal/": false,
	} {
		u, _ := url.Parse(raw)
		if k.Allowed(u) != want {
			t.Errorf("%s allowed=%v", raw, !want)
		}
	}
	if _, err := k.Fetch(context.Background(), "https://example.com/"); err != ErrDenied {
		t.Fatal("non-allowlisted host fetched")
	}
	// even an allowlisted name that resolves to loopback is refused at dial time
	k.Allow = append(k.Allow, "localhost")
	if _, err := k.Fetch(context.Background(), "https://localhost/"); err == nil || !strings.Contains(err.Error(), ErrDenied.Error()) {
		t.Fatalf("loopback dial not blocked: %v", err)
	}
}

func TestAutoIntent(t *testing.T) {
	if len(Auto("Phân tích giá vàng hôm nay (XAU/USD và SJC)")) != 1 || len(Auto("viết thư cho Elena")) != 0 {
		t.Fatal("auto intent")
	}
}

func TestGoldFromFakeFeeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/price/XAU":
			fmt.Fprint(w, `{"price":4000.5,"updatedAt":"2026-10-22T10:00:00Z"}`)
		case strings.Contains(r.URL.Path, "GC=F"):
			fmt.Fprint(w, `{"chart":{"result":[{"meta":{"regularMarketPrice":4010},"timestamp":[1,2,3,4,5,6],"indicators":{"quote":[{"close":[3900,3950,null,3980,4000,4010]}]}}]}}`)
		case strings.Contains(r.URL.Path, "VND=X"):
			fmt.Fprint(w, `{"chart":{"result":[{"meta":{"regularMarketPrice":25000},"timestamp":[1],"indicators":{"quote":[{"close":[25000]}]}}]}}`)
		default:
			r.ParseForm()
			if r.Form.Get("method") == "GetCurrentGoldPricesByBranch" {
				fmt.Fprint(w, `{"success":true,"latestDate":"15:00 22/10/2026","data":[{"TypeName":"Vàng SJC 1L, 10L, 1KG","BuyValue":139000000,"SellValue":142000000}]}`)
			} else {
				fmt.Fprint(w, `{"success":true,"data":[{"SellValue":143000000,"GroupDate":"/Date(1792027884630)/"}]}`)
			}
		}
	}))
	defer srv.Close()
	k := New()
	k.Client = srv.Client()
	k.GoldAPI, k.Yahoo, k.SJC = srv.URL, srv.URL, srv.URL
	k.Now = func() time.Time { return time.Date(2026, 10, 22, 16, 0, 0, 0, time.UTC) }
	r, err := k.Gold(context.Background())
	if err != nil || len(r.Sources) != 4 {
		t.Fatal(err, r.Sources)
	}
	for _, want := range []string{"4000.50 USD/oz", "bán 142.00 triệu", "25000", "Thay đổi 5 phiên: +110.0", "Quy đổi giá thế giới: 120.58"} {
		if !strings.Contains(r.Text, want) {
			t.Errorf("missing %q in\n%s", want, r.Text)
		}
	}
}

func TestGoldReportsMissingInsteadOfGuessing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer srv.Close()
	k := New()
	k.Client = srv.Client()
	k.GoldAPI, k.Yahoo, k.SJC = srv.URL, srv.URL, srv.URL
	r, err := k.Gold(context.Background())
	if err == nil || !strings.Contains(r.Text, "không lấy được") || strings.Contains(r.Text, "Quy đổi") {
		t.Fatalf("%v %s", err, r.Text)
	}
}

func TestPortedOpenDotsHelpers(t *testing.T) {
	for ip, want := range map[string]bool{"8.8.8.8": true, "100.64.1.1": false, "198.18.0.1": false, "192.0.2.5": false, "10.1.1.1": false, "2606:4700::1": true, "fd00::1": false, "::1": false, "2001:db8::1": false} {
		if isPublicAddress(net.ParseIP(ip)) != want {
			t.Errorf("%s", ip)
		}
	}
	got := RequestedURLs("xem (https://vnexpress.net/a_(b)), và [x](https://sjc.com.vn/p). Hết https://vnexpress.net/a_(b)")
	if len(got) != 2 || got[0] != "https://vnexpress.net/a_(b)" || got[1] != "https://sjc.com.vn/p" {
		t.Fatalf("%q", got)
	}
	if u := New().AutoURLs("đọc https://example.com/x và https://vnexpress.net/y."); len(u) != 1 || u[0] != "https://vnexpress.net/y" {
		t.Fatalf("%q", u)
	}
}

func TestFakeIPEgressOnlyRelaxesBenchmarkRange(t *testing.T) {
	defer func(v bool) { fakeIPEgress = v }(fakeIPEgress)
	fakeIPEgress = false
	if blockPrivate("tcp", "198.18.0.1:443", nil) == nil {
		t.Fatal("fake-ip allowed without opt-in")
	}
	fakeIPEgress = true
	if blockPrivate("tcp", "198.18.0.1:443", nil) != nil || blockPrivate("tcp", "127.0.0.1:443", nil) == nil || blockPrivate("tcp", "10.0.0.1:443", nil) == nil {
		t.Fatal("opt-in relaxed more than 198.18/15")
	}
}
