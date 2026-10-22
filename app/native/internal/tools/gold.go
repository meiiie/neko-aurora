package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Gold assembles live gold data from free public endpoints (no key):
// spot XAU/USD (gold-api.com), COMEX GC=F daily closes and USD/VND
// (Yahoo Finance chart API), SJC buy/sell today and the last 8 days
// (sjc.com.vn price service). Each part fails independently and is
// reported as missing rather than guessed.
func (k *Kit) Gold(ctx context.Context) (Result, error) {
	now := k.Now()
	at := now.Format(time.RFC3339)
	loc := time.FixedZone("ICT", 7*3600)
	var (
		mu    sync.Mutex
		parts = map[string]string{}
		srcs  = map[string]Source{}
		wg    sync.WaitGroup
	)
	put := func(key, text string, s Source) {
		mu.Lock()
		parts[key], srcs[key] = text, s
		mu.Unlock()
	}
	wg.Add(4)
	go func() {
		defer wg.Done()
		var j struct {
			Price     float64 `json:"price"`
			UpdatedAt string  `json:"updatedAt"`
		}
		b, err := k.get(ctx, "GET", k.GoldAPI+"/price/XAU", "")
		if err != nil || json.Unmarshal(b, &j) != nil || j.Price <= 0 {
			put("spot", "XAU/USD giao ngay: không lấy được ("+errText(err)+")", Source{})
			return
		}
		put("spot", fmt.Sprintf("XAU/USD giao ngay: %.2f USD/oz (cập nhật %s)", j.Price, ict(j.UpdatedAt, loc)), Source{Title: "Gold API — XAU/USD giao ngay", URL: "https://api.gold-api.com/price/XAU", At: at})
	}()
	go func() {
		defer wg.Done()
		ts, cl, last, err := k.yahoo(ctx, "GC=F", "1mo")
		if err != nil || len(cl) < 2 {
			put("fut", "Vàng tương lai COMEX GC=F: không lấy được ("+errText(err)+")", Source{})
			return
		}
		var sb strings.Builder
		n := len(cl)
		from := max(0, n-8)
		sb.WriteString("Vàng tương lai COMEX (GC=F), giá đóng cửa ngày, USD/oz:")
		for i := from; i < n; i++ {
			fmt.Fprintf(&sb, " %s=%.1f", time.Unix(ts[i], 0).In(loc).Format("02/01"), cl[i])
		}
		wk := max(0, n-6) // ~5 trading days back
		fmt.Fprintf(&sb, ". Giá gần nhất %.1f. Thay đổi 5 phiên: %+.1f (%+.2f%%). Cao/thấp 1 tháng: %.1f / %.1f.", last, cl[n-1]-cl[wk], 100*(cl[n-1]-cl[wk])/cl[wk], maxF(cl), minF(cl))
		put("fut", sb.String(), Source{Title: "Yahoo Finance — Gold futures GC=F (COMEX)", URL: "https://finance.yahoo.com/quote/GC=F/history", At: at})
	}()
	go func() {
		defer wg.Done()
		_, _, last, err := k.yahoo(ctx, "VND=X", "5d")
		if err != nil || last <= 0 {
			put("fx", "Tỷ giá USD/VND: không lấy được", Source{})
			return
		}
		put("fx", fmt.Sprintf("Tỷ giá USD/VND (thị trường quốc tế): %.0f", last), Source{Title: "Yahoo Finance — USD/VND", URL: "https://finance.yahoo.com/quote/VND=X", At: at})
	}()
	go func() {
		defer wg.Done()
		put("sjc", k.sjc(ctx, now, loc), Source{Title: "SJC — Giá vàng niêm yết", URL: "https://sjc.com.vn/gia-vang-online", At: at})
	}()
	wg.Wait()
	// derived: SJC premium over world price converted at USD/VND (1 lượng = 37.5 g = 1.20565 oz)
	var sb strings.Builder
	fmt.Fprintf(&sb, "Dữ liệu vàng lấy lúc %s (giờ Việt Nam):\n", now.In(loc).Format("15:04 02/01/2006"))
	for _, k := range []string{"spot", "fut", "fx", "sjc"} {
		sb.WriteString("- " + parts[k] + "\n")
	}
	if spot, fx, sell := numAfter(parts["spot"], "giao ngay: "), numAfter(parts["fx"], "): "), sjcSell(parts["sjc"]); spot > 0 && fx > 0 && sell > 0 {
		world := spot * 1.20565 * fx
		fmt.Fprintf(&sb, "- Quy đổi giá thế giới: %.2f triệu đồng/lượng (chưa thuế, phí); SJC bán ra cao hơn %.2f triệu/lượng (%.1f%%).\n", world/1e6, (sell-world)/1e6, 100*(sell-world)/world)
	}
	r := Result{Name: "gold_price", Text: sb.String(), Summary: "Giá vàng trực tuyến"}
	for _, k := range []string{"spot", "fut", "fx", "sjc"} {
		if s := srcs[k]; s.URL != "" && !strings.Contains(parts[k], "không lấy được") {
			r.Sources = append(r.Sources, s)
		}
	}
	if len(r.Sources) == 0 {
		return r, fmt.Errorf("gold_sources_unavailable")
	}
	r.Summary = fmt.Sprintf("Giá vàng trực tuyến · %d nguồn", len(r.Sources))
	return r, nil
}

func errText(err error) string {
	if err == nil {
		return "dữ liệu rỗng"
	}
	return cut(err.Error(), 60)
}

func ict(s string, loc *time.Location) string {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.In(loc).Format("15:04 02/01")
	}
	return s
}

func (k *Kit) yahoo(ctx context.Context, sym, rng string) ([]int64, []float64, float64, error) {
	b, err := k.get(ctx, "GET", k.Yahoo+"/v8/finance/chart/"+sym+"?range="+rng+"&interval=1d", "")
	if err != nil {
		return nil, nil, 0, err
	}
	var j struct {
		Chart struct {
			Result []struct {
				Meta struct {
					Price float64 `json:"regularMarketPrice"`
				} `json:"meta"`
				Timestamp  []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Close []*float64 `json:"close"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
		} `json:"chart"`
	}
	if err := json.Unmarshal(b, &j); err != nil || len(j.Chart.Result) == 0 || len(j.Chart.Result[0].Indicators.Quote) == 0 {
		return nil, nil, 0, fmt.Errorf("yahoo_parse")
	}
	r := j.Chart.Result[0]
	var ts []int64
	var cl []float64
	for i, c := range r.Indicators.Quote[0].Close {
		if c != nil && i < len(r.Timestamp) && !math.IsNaN(*c) {
			ts, cl = append(ts, r.Timestamp[i]), append(cl, *c)
		}
	}
	return ts, cl, r.Meta.Price, nil
}

type sjcRow struct {
	TypeName   string  `json:"TypeName"`
	BranchName string  `json:"BranchName"`
	BuyValue   float64 `json:"BuyValue"`
	SellValue  float64 `json:"SellValue"`
	GroupDate  string  `json:"GroupDate"`
}

var msRe = regexp.MustCompile(`Date\((-?\d+)\)`)

func (k *Kit) sjc(ctx context.Context, now time.Time, loc *time.Location) string {
	var cur struct {
		Success    bool     `json:"success"`
		LatestDate string   `json:"latestDate"`
		Data       []sjcRow `json:"data"`
	}
	out := "Vàng miếng SJC: không lấy được"
	if b, err := k.get(ctx, "POST", k.SJC+"/GoldPrice/Services/PriceService.ashx", "method=GetCurrentGoldPricesByBranch&BranchId=1"); err == nil && json.Unmarshal(b, &cur) == nil && cur.Success {
		for _, r := range cur.Data {
			if strings.Contains(r.TypeName, "1L") && r.SellValue > 0 {
				out = fmt.Sprintf("Vàng miếng SJC 1L (TP.HCM) lúc %s: mua %.2f, bán %.2f triệu đồng/lượng", cur.LatestDate, r.BuyValue/1e6, r.SellValue/1e6)
				break
			}
		}
		for _, r := range cur.Data {
			if strings.Contains(strings.ToLower(r.TypeName), "nhẫn") && r.SellValue > 0 {
				out += fmt.Sprintf("; %s: mua %.2f, bán %.2f", r.TypeName, r.BuyValue/1e6, r.SellValue/1e6)
				break
			}
		}
	}
	var hist struct {
		Success bool     `json:"success"`
		Data    []sjcRow `json:"data"`
	}
	from := now.AddDate(0, 0, -8).In(loc).Format("02/01/2006")
	to := now.In(loc).Format("02/01/2006")
	if b, err := k.get(ctx, "POST", k.SJC+"/GoldPrice/Services/PriceService.ashx", "method=GetGoldPriceHistory&goldPriceId=1&fromDate="+from+"&toDate="+to); err == nil && json.Unmarshal(b, &hist) == nil && hist.Success && len(hist.Data) > 0 {
		days := map[string]sjcRow{}
		order := []string{}
		for _, r := range hist.Data {
			m := msRe.FindStringSubmatch(r.GroupDate)
			if m == nil {
				continue
			}
			ms, _ := strconv.ParseInt(m[1], 10, 64)
			d := time.UnixMilli(ms).In(loc).Format("02/01")
			if _, ok := days[d]; !ok {
				order = append(order, d)
			}
			days[d] = r // last entry of the day
		}
		out += ". Lịch sử SJC 1L (giá bán cuối ngày, triệu/lượng):"
		for _, d := range order {
			out += fmt.Sprintf(" %s=%.2f", d, days[d].SellValue/1e6)
		}
	}
	return out
}

func numAfter(s, key string) float64 {
	i := strings.Index(s, key)
	if i < 0 {
		return 0
	}
	f := strings.Fields(s[i+len(key):])
	if len(f) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(f[0], 64)
	return v
}

var sellRe = regexp.MustCompile(`bán ([0-9.]+) triệu`)

func sjcSell(s string) float64 {
	m := sellRe.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	return v * 1e6
}

func maxF(a []float64) float64 {
	m := a[0]
	for _, v := range a {
		m = math.Max(m, v)
	}
	return m
}
func minF(a []float64) float64 {
	m := a[0]
	for _, v := range a {
		m = math.Min(m, v)
	}
	return m
}
