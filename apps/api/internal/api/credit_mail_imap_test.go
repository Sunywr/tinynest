package api

import (
	"bufio"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestLegacyCMBMultipartUTF8Alias(t *testing.T) {
	header := mimeHeader{"Content-Type": {`multipart/mixed; boundary="legacy"`}}
	body := "--legacy\r\nContent-Type: text/html; charset=utf8\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n\r\n" +
		"<table><tr><td>=E8=B4=A6=E5=8D=95</td></tr></table>\r\n--legacy--\r\n"
	got, err := everbrightMIMEHTML(header, strings.NewReader(body), 0)
	if err != nil || !strings.Contains(got, "账单") {
		t.Fatalf("legacy HTML body not decoded: %v", err)
	}
}

func TestCMBReadsHTMLAttachmentWhenInlineHasNoDetails(t *testing.T) {
	const table = "<table><tr><td>0416</td><td>0417</td><td>测试消费</td><td>￥28.80</td><td>0123</td><td>CN</td><td>28.80</td></tr></table>"
	header := mimeHeader{"Content-Type": {`multipart/mixed; boundary="legacy"`}}
	body := "--legacy\r\nContent-Type: text/html; charset=utf8\r\n\r\n<html>账单通知</html>\r\n" +
		"--legacy\r\nContent-Type: text/html; name=cover.htm\r\nContent-Disposition: attachment\r\n\r\n<html>cover</html>\r\n" +
		"--legacy\r\nContent-Type: application/octet-stream; name=statement.htm\r\n" +
		"Content-Disposition: attachment\r\n\r\n" + table + "\r\n--legacy--\r\n"
	date := time.Date(2023, 5, 17, 0, 0, 0, 0, time.UTC)
	got, err := cmbMIMEHTML(header, strings.NewReader(body), date, "0123")
	if err != nil || !strings.Contains(got, table) {
		t.Fatalf("CMB attachment was not selected: %v", err)
	}
}

func TestCITICSearchNarrowsLargeMailboxBeforeFetchingHeaders(t *testing.T) {
	if got := bankStatementIMAPSearch("citic"); got != `UID SEARCH FROM "citic"` {
		t.Fatalf("unexpected search command: %q", got)
	}
}

func TestSPDBSearchNarrowsMailboxAcrossSenderDomainChange(t *testing.T) {
	if got := bankStatementIMAPSearch("spdb"); got != `UID SEARCH FROM "estmtservice"` {
		t.Fatalf("unexpected search command: %q", got)
	}
}

func TestCMBSearchNarrowsLargeMailbox(t *testing.T) {
	if got := bankStatementIMAPSearch("cmb"); got != `UID SEARCH CHARSET UTF-8 SUBJECT "招商银行"` {
		t.Fatalf("unexpected search command: %q", got)
	}
}

func TestCreditIMAPIdentifiesClientBeforeExaminingFolder(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	_ = client.SetDeadline(time.Now().Add(time.Second))
	done := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(server)
		fmt.Fprint(server, "* OK ready\r\n")
		for _, step := range []struct{ command, reply string }{
			{`A001 LOGIN "user" "secret"`, "A001 OK login\r\n"},
			{"A002 CAPABILITY", "* CAPABILITY IMAP4rev1 ID\r\nA002 OK capability\r\n"},
			{`A003 ID ("name" "TinyNest" "version" "1.0" "vendor" "TinyNest")`, "* ID NIL\r\nA003 OK identified\r\n"},
			{`A004 EXAMINE "INBOX"`, "A004 OK [READ-ONLY] opened\r\n"},
		} {
			line, err := reader.ReadString('\n')
			if err != nil || line != step.command+"\r\n" {
				done <- fmt.Errorf("expected %q, got %q (%v)", step.command, line, err)
				server.Close()
				return
			}
			fmt.Fprint(server, step.reply)
		}
		done <- nil
	}()
	if err := probeCreditIMAPSession(client, `"user"`, `"secret"`, `"INBOX"`); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestPublicIMAPAddress(t *testing.T) {
	tests := map[string]bool{
		"8.8.8.8":          true,
		"2606:4700::1111":  true,
		"127.0.0.1":        false,
		"10.0.0.1":         false,
		"100.64.0.1":       false,
		"169.254.169.254":  false,
		"198.18.0.1":       false,
		"::ffff:127.0.0.1": false,
		"64:ff9b::7f00:1":  false,
		"fd00::1":          false,
		"2001:db8::1":      false,
		"2002:7f00:1::":    false,
	}
	for input, want := range tests {
		if got := publicIMAPAddress(netip.MustParseAddr(input)); got != want {
			t.Errorf("publicIMAPAddress(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestQuoteIMAPProbeValue(t *testing.T) {
	value, err := quoteIMAPProbeValue(`a"b\c`)
	if err != nil || value != `"a\"b\\c"` {
		t.Fatalf("unexpected quoted value %q, error %v", value, err)
	}
	if _, err := quoteIMAPProbeValue("INBOX\r\nA003 LOGOUT"); err == nil {
		t.Fatal("expected line break to be rejected")
	}
}

func TestEncodeIMAPFolder(t *testing.T) {
	if got := encodeIMAPFolder("账单&收件箱"); got != "&jSZTVQ-&-&ZTZO9nux-" {
		t.Fatalf("unexpected folder encoding %q", got)
	}
}
