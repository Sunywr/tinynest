package credit

import (
	"fmt"
	"io"
	"mime"
	"net/mail"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/encoding/simplifiedchinese"
)

var everbrightSubjectDate = regexp.MustCompile(`20[0-9]{2}\s*年\s*[0-9]{1,2}\s*月(?:\s*[0-9]{1,2}\s*日)?|\b20[0-9]{2}[-./][0-9]{1,2}(?:[-./][0-9]{1,2})?\b|\b20[0-9]{4}(?:[0-9]{2})?\b`)

// MatchEverbrightStatementHeader matches the configured subject after decoding
// MIME encoded words and removing optional statement dates.
func MatchEverbrightStatementHeader(from, subject, keyword string) bool {
	return MatchBankStatementHeader("ceb", from, subject, keyword)
}

func MatchBankStatementHeader(bank, from, subject, keyword string) bool {
	domains := map[string]string{"ceb": "cardcenter.cebbank.com", "gdb": "cgbchina.com.cn", "comm": "bocomcc.com", "cmb": "message.cmbchina.com"}
	decoder := &mime.WordDecoder{CharsetReader: EverbrightCharsetReader}
	address, err := (&mail.AddressParser{WordDecoder: decoder}).Parse(from)
	if err != nil {
		return false
	}
	_, domain, ok := strings.Cut(address.Address, "@")
	if !ok || domains[bank] == "" || !strings.EqualFold(domain, domains[bank]) {
		return false
	}
	decoded, err := decoder.DecodeHeader(subject)
	if err != nil {
		return false
	}
	normalize := func(value string) string {
		return strings.Map(func(ch rune) rune {
			if unicode.IsSpace(ch) || strings.ContainsRune("()（）[]【】-_—", ch) {
				return -1
			}
			return unicode.ToLower(ch)
		}, everbrightSubjectDate.ReplaceAllString(value, ""))
	}
	want := normalize(keyword)
	return want != "" && strings.Contains(normalize(decoded), want)
}

// EverbrightCharsetReader decodes the UTF-8 and GB18030 encodings observed
// in the bank's statement messages.
func EverbrightCharsetReader(charset string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(charset) {
	case "", "utf-8", "us-ascii":
		return input, nil
	case "gb18030":
		return simplifiedchinese.GB18030.NewDecoder().Reader(input), nil
	case "gbk", "gb2312":
		return simplifiedchinese.GBK.NewDecoder().Reader(input), nil
	default:
		return nil, fmt.Errorf("unsupported statement character encoding")
	}
}
