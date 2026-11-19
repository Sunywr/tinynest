package api

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"
)

var nonPublicIMAPNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
}

var publicIMAPIPv6 = netip.MustParsePrefix("2000::/3")

func publicIMAPAddress(address netip.Addr) bool {
	address = address.Unmap()
	if address.Zone() != "" || !address.IsGlobalUnicast() || address.IsPrivate() ||
		(address.Is6() && !publicIMAPIPv6.Contains(address)) {
		return false
	}
	for _, network := range nonPublicIMAPNetworks {
		if network.Contains(address) {
			return false
		}
	}
	return true
}

// Dial the checked IP directly so a second DNS lookup cannot change the target.
func dialCreditIMAP(ctx context.Context, host string) (*tls.Conn, error) {
	if !validSMTPHost(host) {
		return nil, errors.New("invalid IMAP hostname")
	}
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addresses) == 0 {
		return nil, errors.New("IMAP hostname resolution failed")
	}
	for _, address := range addresses {
		if !publicIMAPAddress(address) {
			return nil, errors.New("IMAP hostname resolves to a prohibited address")
		}
	}
	dialer := tls.Dialer{
		NetDialer: &net.Dialer{Timeout: 5 * time.Second},
		Config:    &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12},
	}
	for _, address := range addresses {
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(address.String(), "993"))
		if err == nil {
			return conn.(*tls.Conn), nil
		}
	}
	return nil, errors.New("IMAP TLS connection failed")
}

func quoteIMAPProbeValue(value string) (string, error) {
	if value == "" || len(value) > 1024 || strings.ContainsFunc(value, func(r rune) bool {
		return unicode.IsControl(r) || r > unicode.MaxASCII
	}) {
		return "", errors.New("invalid IMAP connection parameter")
	}
	return "\"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(value) + "\"", nil
}

// IMAP4rev1 mailbox names use modified UTF-7 before UTF-8 is negotiated.
func encodeIMAPFolder(folder string) string {
	var result strings.Builder
	var pending []rune
	flush := func() {
		if len(pending) == 0 {
			return
		}
		var raw []byte
		for _, unit := range utf16.Encode(pending) {
			raw = binary.BigEndian.AppendUint16(raw, unit)
		}
		result.WriteByte('&')
		result.WriteString(strings.ReplaceAll(base64.RawStdEncoding.EncodeToString(raw), "/", ","))
		result.WriteByte('-')
		pending = pending[:0]
	}
	for _, ch := range folder {
		if ch > unicode.MaxASCII {
			pending = append(pending, ch)
			continue
		}
		flush()
		result.WriteRune(ch)
		if ch == '&' {
			result.WriteByte('-')
		}
	}
	flush()
	return result.String()
}

// The probe uses EXAMINE to verify folder access without changing message flags.
func probeCreditIMAP(ctx context.Context, host, username, password, folder string) error {
	user, userErr := quoteIMAPProbeValue(username)
	secret, secretErr := quoteIMAPProbeValue(password)
	mailbox, folderErr := quoteIMAPProbeValue(encodeIMAPFolder(folder))
	if userErr != nil || secretErr != nil || folderErr != nil {
		return errors.New("invalid IMAP connection parameters")
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	conn, err := dialCreditIMAP(ctx, host)
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	reader := bufio.NewReaderSize(conn, 16*1024)
	greeting, err := reader.ReadSlice('\n')
	if err != nil || !strings.HasPrefix(strings.ToUpper(string(greeting)), "* OK ") {
		return errors.New("invalid IMAP greeting")
	}
	command := func(tag, value string) bool {
		if _, err := fmt.Fprintf(conn, "%s %s\r\n", tag, value); err != nil {
			return false
		}
		for range 128 {
			line, err := reader.ReadSlice('\n')
			if err != nil {
				return false
			}
			if strings.HasPrefix(string(line), tag+" ") {
				return strings.HasPrefix(strings.ToUpper(string(line)), tag+" OK ")
			}
			if !strings.HasPrefix(string(line), "* ") {
				return false
			}
		}
		return false
	}
	if !command("A001", "LOGIN "+user+" "+secret) {
		return errors.New("IMAP authentication failed")
	}
	if !command("A002", "EXAMINE "+mailbox) {
		return errors.New("IMAP folder access failed")
	}
	return nil
}
