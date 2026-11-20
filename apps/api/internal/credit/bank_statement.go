package credit

import (
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
)

var maskedGuangfaCard = regexp.MustCompile(`卡号\s*[:：]\s*[0-9]{4}[＊*]{4,12}([0-9]{4})(?:[^0-9]|$)`)

func ParseBankStatement(bank, body string, date time.Time, tail string) ([]EverbrightTransaction, error) {
	if bank == "ceb" {
		return ParseEverbrightHTML(body, date, tail)
	}
	if len(body) > 2<<20 || date.IsZero() || len(tail) != 4 || strings.Trim(tail, "0123456789") != "" {
		return nil, errors.New("invalid bank statement")
	}
	markup := statementTableMarkup(body)
	dec := xml.NewDecoder(strings.NewReader(markup))
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	var tables []everbrightTable
	var result []EverbrightTransaction
	var section string
	var anchors [][]int
	if bank == "gdb" {
		anchors = maskedGuangfaCard.FindAllStringSubmatchIndex(markup, -1)
	}
	guangfaDetails := false
	for {
		token, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("invalid statement HTML")
		}
		if element, ok := token.(xml.StartElement); ok && element.Name.Local == "table" {
			tables = append(tables, everbrightTable{})
			continue
		}
		if len(tables) == 0 {
			continue
		}
		table := &tables[len(tables)-1]
		switch t := token.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "tr":
				table.row = nil
			case "td", "th":
				table.cell = new(strings.Builder)
			}
		case xml.CharData:
			if table.cell != nil {
				table.cell.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "td", "th":
				if table.cell != nil {
					cell := strings.Join(strings.Fields(table.cell.String()), " ")
					table.row = append(table.row, cell)
					table.cell = nil
				}
			case "table":
				tables = tables[:len(tables)-1]
			case "tr":
				for len(anchors) > 0 && int64(anchors[0][3]) < dec.InputOffset() {
					section = markup[anchors[0][2]:anchors[0][3]]
					guangfaDetails = false
					anchors = anchors[1:]
				}
				row := table.row
				var occurred, posted, description, amount string
				switch {
				case bank == "gdb" && len(row) == 7:
					if strings.Contains(row[0], "交易日期") && strings.Contains(row[2], "交易摘要") && strings.Contains(row[5], "入账金额") && strings.Contains(row[6], "入账货币") {
						guangfaDetails = section == tail
						continue
					}
					if !guangfaDetails || section != tail || row[6] != "人民币" || len(row[0]) != 10 || len(row[1]) != 10 {
						continue
					}
					occurred, posted, description, amount = row[0], row[1], row[2], row[5]
				case bank == "comm" && len(row) == 7 && row[3] == tail:
					if !strings.HasPrefix(row[6], "CNY ") {
						continue
					}
					occurred, posted, description, amount = row[1], row[2], row[4], strings.TrimPrefix(row[6], "CNY ")
				case bank == "cmb" && len(row) == 8 && row[5] == tail:
					if !strings.HasPrefix(row[4], "¥") && !strings.HasPrefix(row[4], "￥") {
						continue
					}
					occurred, posted, description, amount = row[1], row[2], row[3], row[4]
				default:
					continue
				}
				postedOn, err := bankStatementDate(posted, date)
				if err != nil {
					return nil, err
				}
				transOn := postedOn
				if occurred != "" {
					transOn, err = bankStatementDate(occurred, postedOn)
					if err != nil {
						return nil, err
					}
				}
				minor, err := parseEverbrightAmount(amount)
				if err != nil || description == "" {
					return nil, errors.New("invalid bank transaction row")
				}
				if minor == 0 {
					continue
				}
				result = append(result, EverbrightTransaction{TransactionDate: transOn.Format(time.DateOnly), PostingDate: postedOn.Format(time.DateOnly), CardTail: tail, Description: description, Amount: minor})
			}
		}
	}
	if len(result) == 0 {
		return nil, errors.New("no matching RMB statement details")
	}
	return result, nil
}

func bankStatementDate(raw string, upper time.Time) (time.Time, error) {
	if len(raw) == 4 && strings.Trim(raw, "0123456789") == "" {
		raw = raw[:2] + "/" + raw[2:]
	}
	return everbrightDate(raw, upper)
}
