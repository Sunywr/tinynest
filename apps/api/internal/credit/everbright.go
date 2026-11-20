package credit

import (
	"encoding/xml"
	"fmt"
	stdhtml "html"
	"io"
	"regexp"
	"strings"
	"time"

	"tinynest/api/internal/money"
)

// EverbrightTransaction is a transaction extracted from the RMB detail table.
type EverbrightTransaction struct {
	TransactionDate string
	PostingDate     string
	CardTail        string
	Description     string
	Amount          money.Minor
}

type everbrightTable struct {
	row     []string
	cell    *strings.Builder
	details bool
}

var (
	everbrightNonContent = regexp.MustCompile(`(?is)<!--.*?-->|<script\b[^>]*>.*?</script\s*>|<style\b[^>]*>.*?</style\s*>`)
	everbrightHTMLTag    = regexp.MustCompile(`(?i)<(/?)([a-z][a-z0-9:-]*)\b[^>]*>`)
)

// ParseEverbrightHTML reads the observed Everbright email detail table. statementDate
// is the verified closing date, needed to resolve older month/day-only templates.
func ParseEverbrightHTML(body string, statementDate time.Time, cardTail string) ([]EverbrightTransaction, error) {
	if len(body) > 2<<20 || statementDate.IsZero() || len(cardTail) != 4 || strings.Trim(cardTail, "0123456789") != "" {
		return nil, fmt.Errorf("invalid Everbright statement input")
	}
	// Keep table structure while dropping presentation markup and its attributes.
	// Some templates contain unbalanced quotes in inline styles.
	dec := xml.NewDecoder(strings.NewReader(statementTableMarkup(body)))
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	return parseEverbrightTable(dec, statementDate, cardTail)
}

func statementTableMarkup(body string) string {
	body = everbrightNonContent.ReplaceAllString(body, "")
	return everbrightHTMLTag.ReplaceAllStringFunc(body, func(tag string) string {
		parts := everbrightHTMLTag.FindStringSubmatch(tag)
		switch name := strings.ToLower(parts[2]); name {
		case "table", "tr", "td", "th":
			return "<" + parts[1] + name + ">"
		case "br":
			return " "
		default:
			return ""
		}
	})
}

func parseEverbrightTable(dec *xml.Decoder, statementDate time.Time, cardTail string) ([]EverbrightTransaction, error) {
	var tables []everbrightTable
	var transactions []EverbrightTransaction
	found := false
	for {
		// Email clients tolerate unclosed presentation tags. RawToken lets the
		// table reader pair only the structural tags it actually uses.
		token, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("could not read Everbright HTML")
		}
		if tag, ok := token.(xml.StartElement); ok && strings.EqualFold(tag.Name.Local, "table") {
			tables = append(tables, everbrightTable{})
			continue
		}
		if len(tables) == 0 {
			continue
		}
		table := &tables[len(tables)-1]
		switch t := token.(type) {
		case xml.StartElement:
			switch strings.ToLower(t.Name.Local) {
			case "tr":
				table.row = nil
			case "td", "th":
				table.cell = new(strings.Builder)
			}
		case xml.CharData:
			if table.cell != nil {
				table.cell.Write([]byte(t))
			}
		case xml.EndElement:
			switch strings.ToLower(t.Name.Local) {
			case "td", "th":
				if table.cell != nil {
					table.row = append(table.row, strings.Join(strings.Fields(stdhtml.UnescapeString(table.cell.String())), " "))
					table.cell = nil
				}
			case "tr":
				row := table.row
				if len(row) != 5 {
					continue
				}
				if strings.Contains(row[0], "交易日") && strings.Contains(row[1], "记账日") && strings.Contains(strings.ReplaceAll(row[2], " ", ""), "卡号末四位") && strings.Contains(row[3], "交易说明") {
					table.details = strings.Contains(strings.ToUpper(row[4]), "RMB") || strings.Contains(row[4], "人民币")
					if table.details && found {
						return nil, fmt.Errorf("multiple Everbright RMB detail tables")
					}
					found = found || table.details
					continue
				}
				if !table.details || row[2] != cardTail {
					continue
				}
				posted, err := everbrightDate(row[1], statementDate)
				if err != nil {
					return nil, err
				}
				transacted, err := everbrightDate(row[0], posted)
				if err != nil {
					return nil, err
				}
				amount, err := parseEverbrightAmount(row[4])
				if err != nil || amount == 0 || row[3] == "" {
					return nil, fmt.Errorf("invalid Everbright transaction detail")
				}
				transactions = append(transactions, EverbrightTransaction{
					TransactionDate: transacted.Format(time.DateOnly), PostingDate: posted.Format(time.DateOnly),
					CardTail: cardTail, Description: row[3], Amount: amount,
				})
			case "table":
				tables = tables[:len(tables)-1]
			}
		}
	}
	if !found {
		return nil, fmt.Errorf("Everbright RMB transaction table not found")
	}
	return transactions, nil
}

func everbrightDate(raw string, upper time.Time) (time.Time, error) {
	limit := time.Date(upper.Year(), upper.Month(), upper.Day(), 0, 0, 0, 0, time.UTC)
	layouts := []string{"2006/01/02", "2006-01-02"}
	candidates := []string{raw}
	if len(raw) == 5 {
		candidates = []string{fmt.Sprintf("%04d/%s", upper.Year(), raw), fmt.Sprintf("%04d/%s", upper.Year()-1, raw)}
	}
	for _, value := range candidates {
		for _, layout := range layouts {
			day, err := time.Parse(layout, value)
			if err == nil && !day.After(limit) {
				return day, nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("invalid Everbright transaction date")
}

func parseEverbrightAmount(raw string) (money.Minor, error) {
	value := strings.NewReplacer(" ", "", "¥", "", "￥", "", "−", "-").Replace(raw)
	if credit, ok := strings.CutPrefix(value, "(存入)"); ok {
		if credit == "" || strings.HasPrefix(credit, "+") || strings.HasPrefix(credit, "-") {
			return 0, money.ErrInvalidAmount
		}
		value = "-" + credit
	}
	if strings.Contains(value, ",") {
		integer, fraction, _ := strings.Cut(strings.TrimLeft(value, "+-"), ".")
		if strings.Contains(fraction, ",") {
			return 0, money.ErrInvalidAmount
		}
		groups := strings.Split(integer, ",")
		if len(groups[0]) < 1 || len(groups[0]) > 3 {
			return 0, money.ErrInvalidAmount
		}
		for _, group := range groups[1:] {
			if len(group) != 3 {
				return 0, money.ErrInvalidAmount
			}
		}
		value = strings.ReplaceAll(value, ",", "")
	}
	return money.ParseYuan(value)
}
