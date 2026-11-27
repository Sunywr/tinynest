package api

import (
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type marketSource struct {
	referer string
	queries string
}

var marketSources = map[string]marketSource{
	"https://push2.eastmoney.com/api/qt/ulist.np/get":                                           {"https://quote.eastmoney.com/", "fltt invt secids fields ut"},
	"https://push2his.eastmoney.com/api/qt/stock/kline/get":                                     {"https://quote.eastmoney.com/", "secid klt fqt beg end lmt fields1 fields2 ut"},
	"https://datacenter-web.eastmoney.com/api/data/v1/get":                                      {"https://quote.eastmoney.com/", "reportName columns filter sortColumns sortTypes pageSize pageNumber source client"},
	"https://web.ifzq.gtimg.cn/appstock/app/kline/kline":                                        {"https://gu.qq.com/", "param"},
	"https://web.ifzq.gtimg.cn/appstock/app/fqkline/get":                                        {"https://gu.qq.com/", "param"},
	"https://quotes.sina.cn/cn/api/jsonp_v2.php/var%20kline=/CN_MarketDataService.getKLineData": {"https://finance.sina.com.cn/", "symbol scale ma datalen"},
	"https://www.csindex.com.cn/csindex-home/perf/index-perf":                                   {"https://www.csindex.com.cn/", "indexCode startDate endDate"},
	"https://www.gov.cn/zhengce/zuixin/":                                                        {},
	"https://www.ndrc.gov.cn/xwdt/xwfb/wap_index.html":                                          {},
	"https://www.ndrc.gov.cn/xxgk/jd/jd/":                                                       {},
	"https://www.miit.gov.cn/":                                                                  {},
	"https://www.nea.gov.cn/":                                                                   {},
	"https://www.mofcom.gov.cn/zcfb/":                                                           {},
	"https://www.federalreserve.gov/feeds/press_all.xml":                                        {},
	"https://ec.europa.eu/commission/presscorner/api/rss":                                       {"", "language"},
	"https://www.eia.gov/rss/todayinenergy.xml":                                                 {},
}

var (
	marketQuotePath = regexp.MustCompile(`^/q=(?:(?:sh|sz)[0-9]{6},){0,49}(?:sh|sz)[0-9]{6}$`)
	marketFundPath  = regexp.MustCompile(`^/fhsp_[0-9]{6}\.html$`)
	marketValue     = regexp.MustCompile(`^[a-zA-Z0-9_,.()="<>=-]{1,550}$`)
	marketReports   = regexp.MustCompile(`^RPT_(SHAREBONUS_DET|LICO_FN_CPD)$`)
	marketFilter    = regexp.MustCompile(`^\(SECURITY_CODE="[0-9]{6}"\)$|^\(EX_DIVIDEND_DATE>="[0-9-]{10}"\)\(EX_DIVIDEND_DATE<="[0-9-]{10}"\)$`)
	marketPage      = regexp.MustCompile(`^(?:[1-9][0-9]?|1[0-9]{2}|200)$`)
	marketSlots     = make(chan struct{}, 32)
	marketClient    = &http.Client{
		Timeout: 12 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
)

func allowedMarketURL(raw string) (*url.URL, marketSource, bool) {
	if len(raw) > 1000 {
		return nil, marketSource{}, false
	}
	target, err := url.Parse(raw)
	if err != nil || target.Scheme != "https" || target.User != nil || target.Fragment != "" {
		return nil, marketSource{}, false
	}
	source, ok := marketSources["https://"+target.Host+target.EscapedPath()]
	switch {
	case target.Host == "qt.gtimg.cn" && marketQuotePath.MatchString(target.EscapedPath()):
		source, ok = marketSource{referer: "https://gu.qq.com/"}, true
	case target.Host == "fundf10.eastmoney.com" && marketFundPath.MatchString(target.EscapedPath()):
		ok = true
	}
	query, err := url.ParseQuery(target.RawQuery)
	if !ok || err != nil || len(query) != len(strings.Fields(source.queries)) {
		return nil, marketSource{}, false
	}
	for _, key := range strings.Fields(source.queries) {
		values := query[key]
		if len(values) != 1 || !marketValue.MatchString(values[0]) {
			return nil, marketSource{}, false
		}
	}
	if target.Host == "datacenter-web.eastmoney.com" &&
		(!marketReports.MatchString(query.Get("reportName")) || !marketFilter.MatchString(query.Get("filter")) ||
			!marketPage.MatchString(query.Get("pageSize")) || !marketPage.MatchString(query.Get("pageNumber"))) {
		return nil, marketSource{}, false
	}
	return target, source, true
}

func (a *API) handleStockMarket(w http.ResponseWriter, r *http.Request) {
	target, source, ok := allowedMarketURL(r.URL.Query().Get("source"))
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid_market_source", "行情来源无效")
		return
	}
	select {
	case marketSlots <- struct{}{}:
		defer func() { <-marketSlots }()
	default:
		writeErr(w, http.StatusTooManyRequests, "market_busy", "行情请求繁忙，请稍后重试")
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target.String(), nil)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_market_source", "行情来源无效")
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json,text/html,application/xml,application/rss+xml,*/*")
	if source.referer != "" {
		req.Header.Set("Referer", source.referer)
	}
	response, err := marketClient.Do(req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "market_unavailable", "行情来源暂不可用")
		return
	}
	defer response.Body.Close()
	const maxBody = 2 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if response.StatusCode != http.StatusOK || err != nil || len(body) > maxBody {
		writeErr(w, http.StatusBadGateway, "market_unavailable", "行情来源暂不可用")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}
