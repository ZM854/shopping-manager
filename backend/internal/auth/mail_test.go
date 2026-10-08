package auth

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	netmail "net/mail"
	"strings"
	"testing"
	"time"
)

type smtpResult struct {
	body string
	err  error
}

func localSMTP(t *testing.T, reject bool) (int, <-chan smtpResult) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	result := make(chan smtpResult, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			result <- smtpResult{err: err}
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		reader := bufio.NewReader(conn)
		fmt.Fprint(conn, "220 localhost ESMTP test\r\n")
		var message strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				result <- smtpResult{err: err}
				return
			}
			command := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(command, "EHLO"), strings.HasPrefix(command, "HELO"):
				fmt.Fprint(conn, "250-localhost\r\n250 8BITMIME\r\n")
			case strings.HasPrefix(command, "MAIL"), strings.HasPrefix(command, "RCPT"):
				if reject {
					fmt.Fprint(conn, "550 rejected by local test server\r\n")
					result <- smtpResult{}
					return
				}
				fmt.Fprint(conn, "250 OK\r\n")
			case command == "DATA":
				fmt.Fprint(conn, "354 End with .\r\n")
				for {
					data, err := reader.ReadString('\n')
					if err != nil {
						result <- smtpResult{err: err}
						return
					}
					if data == ".\r\n" {
						break
					}
					message.WriteString(data)
				}
				fmt.Fprint(conn, "250 Queued\r\n")
			case command == "QUIT":
				fmt.Fprint(conn, "221 Bye\r\n")
				result <- smtpResult{body: message.String()}
				return
			case command == "RSET", command == "NOOP":
				fmt.Fprint(conn, "250 OK\r\n")
			default:
				fmt.Fprint(conn, "500 Unsupported\r\n")
			}
		}
	}()
	return listener.Addr().(*net.TCPAddr).Port, result
}
func TestActivationMailThroughLocalSMTP(t *testing.T) {
	port, result := localSMTP(t, false)
	service, err := NewMailService(testLog(), "127.0.0.1", port, "", "", "sender@example.test", false, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	link := "https://example.test/api/activate/test-token"
	if err := service.SendActivationMail(ctx, "receiver@example.test", link); err != nil {
		t.Fatal(err)
	}
	select {
	case received := <-result:
		if received.err != nil {
			t.Fatal(received.err)
		}
		message, err := netmail.ReadMessage(strings.NewReader(received.body))
		if err != nil {
			t.Fatal(err)
		}
		subject, err := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
		if err != nil || subject != "Активация аккаунта" {
			t.Fatal("Incorrect activation subject")
		}
		from, _ := netmail.ParseAddress(message.Header.Get("From"))
		to, _ := netmail.ParseAddress(message.Header.Get("To"))
		if from == nil || to == nil || from.Address != "sender@example.test" || to.Address != "receiver@example.test" {
			t.Fatal("Wrong SMTP addresses")
		}
		var body io.Reader = message.Body
		switch strings.ToLower(message.Header.Get("Content-Transfer-Encoding")) {
		case "quoted-printable":
			body = quotedprintable.NewReader(body)
		case "base64":
			body = base64.NewDecoder(base64.StdEncoding, body)
		}
		data, err := io.ReadAll(body)
		if err != nil || !strings.Contains(string(data), link) {
			t.Fatal("Activation link missing from mail")
		}
	case <-ctx.Done():
		t.Fatal("Local SMTP did not deliver result")
	}
}
func TestMailValidationAndTransportErrors(t *testing.T) {
	service, err := NewMailService(testLog(), "127.0.0.1", 1025, "", "", "sender@example.test", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SendActivationMail(context.Background(), "invalid-address", "https://example.test"); err == nil {
		t.Fatal("Invalid recipient accepted")
	}
	service.from = "invalid-address"
	if err := service.SendActivationMail(context.Background(), "receiver@example.test", "https://example.test"); err == nil {
		t.Fatal("Invalid sender accepted")
	}
	port, _ := localSMTP(t, true)
	service, err = NewMailService(testLog(), "127.0.0.1", port, "", "", "sender@example.test", false, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := service.SendActivationMail(ctx, "receiver@example.test", "https://example.test"); err == nil {
		t.Fatal("SMTP rejection must be propagated")
	}
	if _, err := NewMailService(testLog(), "127.0.0.1", -1, "test", "test-only", "sender@example.test", true, true); err == nil {
		t.Fatal("Invalid SMTP port accepted")
	}
}
