package api

import (
	"net/http"
	"net/url"
	"testing"
)

func TestStockMarketSourceAllowlist(t *testing.T) {
	for _, source := range []string{
		"https://qt.gtimg.cn/q=sh000001,sz399001,sz399006",
		"https://fundf10.eastmoney.com/fhsp_510300.html",
		"https://quotes.sina.cn/cn/api/jsonp_v2.php/var%20kline=/CN_MarketDataService.getKLineData?symbol=sh600900&scale=240&ma=no&datalen=75",
		"https://datacenter-web.eastmoney.com/api/data/v1/get?reportName=RPT_SHAREBONUS_DET&columns=ALL&filter=%28SECURITY_CODE%3D%22600900%22%29&sortColumns=EX_DIVIDEND_DATE&sortTypes=-1&pageSize=80&pageNumber=1&source=WEB&client=WEB",
		"https://ec.europa.eu/commission/presscorner/api/rss?language=en",
	} {
		if _, _, ok := allowedMarketURL(source); !ok {
			t.Errorf("rejected configured market source %s", source)
		}
	}
	for _, source := range []string{
		"http://qt.gtimg.cn/q=sh000001",
		"https://qt.gtimg.cn.example.org/q=sh000001",
		"https://qt.gtimg.cn:443/q=sh000001",
		"https://user@qt.gtimg.cn/q=sh000001",
		"https://qt.gtimg.cn/q=sh000001?redirect=http%3A%2F%2F127.0.0.1",
		"https://qt.gtimg.cn/q=sh000001%2F..%2F",
		"https://qt.gtimg.cn/q=sh000001#tag",
		"https://ec.europa.eu/commission/presscorner/api/rss?language=en&language=fr",
		"https://www.miit.gov.cn/other/path",
	} {
		if _, _, ok := allowedMarketURL(source); ok {
			t.Errorf("accepted unconfigured market source %s", source)
		}
	}
}

func TestStockMarketRequiresSessionAndRejectsUnknownSource(t *testing.T) {
	ts, client, database := newTestServer(t)
	defer database.Close()
	defer ts.Close()
	path := "/api/v1/stock-market?source=" + url.QueryEscape("https://example.org/")
	response, err := ts.Client().Get(ts.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous request returned %d", response.StatusCode)
	}
	if code, _ := client.do(http.MethodGet, path, nil); code != http.StatusBadRequest {
		t.Fatalf("unknown market URL returned %d", code)
	}
}
