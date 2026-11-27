package api

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxStatementMailBytes = 512 << 10

var statementFetchLiteral = regexp.MustCompile(`(?i)^\* [0-9]+ FETCH \((?:UID ([0-9]+) )?BODY\[\](?:<0>)? \{([0-9]+)\}\r\n$`)

// readStatementInbox searches the bound card's bank mail read-only and returns
// every message the parser recognised. The preview endpoint reports the result
// and the sync endpoint stores it; neither may write to the mailbox.
func readStatementInbox(ctx context.Context, email, code, sender, bank, tail string, since time.Time) ([]statementMailPreview, error) {
	return readStatementInboxWindow(ctx, email, code, sender, bank, tail, statementDateRange{start: since})
}

func readStatementInboxWindow(ctx context.Context, email, code, sender, bank, tail string, period statementDateRange) ([]statementMailPreview, error) {
	items := []statementMailPreview{}
	err := withStatementInbox(ctx, email, code, 15*time.Second, func(conn io.Writer, reader *bufio.Reader) error {
		uids, err := searchStatementMail(conn, reader, sender, period.start, period.end)
		if err != nil {
			return err
		}
		for _, uid := range uids {
			raw, err := fetchStatementMail(conn, reader, uid)
			if err != nil {
				return err
			}
			if item, ok := parseStatementMail(raw, bank, tail); ok {
				item.UID = uid
				items = append(items, item)
			}
		}
		return nil
	})
	return items, err
}

func searchStatementMail(conn io.Writer, reader *bufio.Reader, sender string, since time.Time, end ...time.Time) ([]uint32, error) {
	before := ""
	limit := 8
	if len(end) > 0 && !end[0].IsZero() {
		before = " BEFORE " + end[0].AddDate(0, 0, 8).Format("02-Jan-2006")
		limit = 64
	}
	if _, err := fmt.Fprintf(conn, "S1 UID SEARCH SINCE %s%s FROM %q\r\n", since.Format("02-Jan-2006"), before, sender); err != nil {
		return nil, err
	}
	uids := []uint32{}
	for i := 0; i < 32; i++ {
		raw, err := reader.ReadSlice('\n')
		if err != nil {
			return nil, err
		}
		line := string(raw)
		if imapStatus(line, "S1", "OK") {
			sort.Slice(uids, func(i, j int) bool { return uids[i] > uids[j] })
			if len(uids) > limit {
				uids = uids[:limit]
			}
			return uids, nil
		}
		if imapStatus(line, "*", "BYE") || !strings.HasPrefix(line, "* ") {
			break
		}
		if len(uids) == 0 && imapStatus(line, "*", "SEARCH") {
			for _, field := range strings.Fields(line)[2:] {
				uid, err := strconv.ParseUint(field, 10, 32)
				if err != nil || uid == 0 {
					return nil, errors.New("invalid statement IMAP search result")
				}
				uids = append(uids, uint32(uid))
			}
		}
	}
	return nil, errors.New("statement IMAP search rejected")
}

func fetchStatementMail(conn io.Writer, reader *bufio.Reader, uid uint32) ([]byte, error) {
	if _, err := fmt.Fprintf(conn, "F1 UID FETCH %d (UID BODY.PEEK[]<0.%d>)\r\n", uid, maxStatementMailBytes+1); err != nil {
		return nil, err
	}
	var body []byte
	wantUID := strconv.FormatUint(uint64(uid), 10)
	for i := 0; i < 32; i++ {
		raw, err := reader.ReadSlice('\n')
		if err != nil {
			return nil, err
		}
		line := string(raw)
		if imapStatus(line, "F1", "OK") {
			return body, nil
		}
		match := statementFetchLiteral.FindStringSubmatch(line)
		if match != nil {
			size, err := strconv.Atoi(match[2])
			if err != nil || size > maxStatementMailBytes+1 || body != nil || (match[1] != "" && match[1] != wantUID) {
				return nil, errors.New("invalid statement IMAP fetch result")
			}
			body = make([]byte, size)
			if _, err := io.ReadFull(reader, body); err != nil {
				return nil, err
			}
			closeLine, err := reader.ReadSlice('\n')
			expected := ")\r\n"
			if match[1] == "" {
				expected = " UID " + wantUID + expected
			}
			if err != nil || !strings.EqualFold(string(closeLine), expected) {
				return nil, errors.New("invalid statement IMAP fetch suffix")
			}
			continue
		}
		if imapStatus(line, "*", "BYE") || !strings.HasPrefix(line, "* ") || strings.HasSuffix(line, "}\r\n") {
			break
		}
	}
	return nil, errors.New("statement IMAP fetch rejected")
}
