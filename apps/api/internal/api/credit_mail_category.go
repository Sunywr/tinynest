package api

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

type statementCategoryRule struct {
	names []string
	terms []string
}

var statementCategoryRules = []statementCategoryRule{
	{[]string{"饮食", "餐饮", "外卖"}, []string{
		"餐厅", "餐饮", "饮品店", "奶茶", "咖啡店", "面包店", "小吃店", "火锅店", "快餐店",
		"肯德基", "麦当劳", "星巴克", "瑞幸", "喜茶", "奈雪", "蜜雪冰城", "霸王茶姬", "茶百道", "古茗",
		"饿了么", "上海拉扎斯", "美团外卖",
	}},
	{[]string{"零食"}, []string{"来伊份", "良品铺子", "三只松鼠", "零食很忙"}},
	{[]string{"交通", "出行"}, []string{"滴滴出行", "高德打车", "曹操出行", "地铁", "公交", "铁路12306", "中国铁路"}},
	{[]string{"汽车", "用车"}, []string{"加油站", "中石油加油", "中石化加油", "充电桩", "停车费", "高速通行费"}},
	{[]string{"服饰", "服装"}, []string{"优衣库", "服饰", "服装"}},
	{[]string{"医疗", "医药"}, []string{"医院", "药房", "大药房", "诊所"}},
	{[]string{"生活", "日用"}, []string{"水费", "电费", "燃气费", "超市", "话费充值", "便利店", "罗森便利", "全家便利"}},
	{[]string{"娱乐", "休闲"}, []string{"电影院", "电影票", "剧院"}},
	{[]string{"旅行", "旅游"}, []string{"酒店", "宾馆", "机票"}},
	{[]string{"运动", "健身"}, []string{"健身房", "羽毛球馆", "游泳馆"}},
	{[]string{"游戏"}, []string{"腾讯游戏", "网易游戏", "steamgames"}},
}

func statementMerchant(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(norm.NFKC.String(value)), ""))
}

// History wins only when all entries for that description agree. Keyword
// rules resolve to an existing category and abstain on any conflicting match.
func suggestStatementCategory(description string, history map[string]string, active map[string][]string) (id, source string) {
	merchant := statementMerchant(description)
	if known, ok := history[merchant]; ok {
		if known != "" {
			return known, "history"
		}
		return "", ""
	}
	for _, rule := range statementCategoryRules {
		hit := false
		for _, term := range rule.terms {
			if strings.Contains(merchant, term) {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		for _, name := range rule.names {
			for _, category := range active[name] {
				if id != "" && id != category {
					return "", ""
				}
				id = category
			}
		}
	}
	if id != "" {
		return id, "rule"
	}
	return "", ""
}
