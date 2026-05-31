// inject — send raw .eml files to the MAILAROO SMTP server for testing.
//
// Usage:
//
//	go run ./cmd/inject email.eml
//	go run ./cmd/inject *.eml
//	go run ./cmd/inject --from sender@example.com --to rcpt@yourdomain.com email.eml
//	go run ./cmd/inject --host localhost --port 2525 email.eml
//	go run ./cmd/inject --mbox mailbox.mbox
//
// The script reads From/To from email headers automatically if not overridden.
// The recipient must match a mailbox address mapping in the database.
// Mbox files are auto-detected when the first line starts with "From ";
// use --mbox to force mbox parsing for files with a different extension.
package main

import (
	"bytes"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

const workers = 5

type job struct {
	raw   []byte
	label string
}

func main() {
	from := flag.String("from", "", "Override MAIL FROM address")
	to := flag.String("to", "", "Override RCPT TO address (comma-separated)")
	host := flag.String("host", "localhost", "SMTP host")
	port := flag.String("port", "2525", "SMTP port")
	dryRun := flag.Bool("dry-run", false, "Parse and display without sending")
	mboxFlag := flag.Bool("mbox", false, "Parse files as mbox format")
	flag.Parse()

	files := flag.Args()
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "usage: inject [flags] <email.eml> ...")
		flag.PrintDefaults()
		os.Exit(1)
	}

	var (
		ok       atomic.Int64
		errCount atomic.Int64
		wg       sync.WaitGroup
		jobs     = make(chan job, len(files))
	)

	for range workers {
		wg.Go(func() {
			for j := range jobs {
				fmt.Printf("\n%s\n", j.label)
				if err := injectRaw(j.raw, *from, *to, *host, *port, *dryRun); err != nil {
					fmt.Printf("  ERROR: %v\n", err)
					errCount.Add(1)
				} else {
					ok.Add(1)
				}
			}
		})
	}

	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading %s: %v\n", path, err)
			errCount.Add(1)
			continue
		}

		isMbox := *mboxFlag || strings.HasSuffix(path, ".mbox") || isMboxContent(raw)
		if isMbox {
			msgs := splitMbox(raw)
			if len(msgs) == 0 {
				fmt.Fprintf(os.Stderr, "no messages found in %s\n", path)
				errCount.Add(1)
				continue
			}
			for i, msg := range msgs {
				jobs <- job{raw: msg, label: fmt.Sprintf("%s [message %d/%d]", path, i+1, len(msgs))}
			}
		} else {
			jobs <- job{raw: raw, label: path}
		}
	}
	close(jobs)
	wg.Wait()

	fmt.Printf("\n%d sent, %d failed\n", ok.Load(), errCount.Load())
	if errCount.Load() > 0 {
		os.Exit(1)
	}
}

func injectRaw(raw []byte, fromFlag, toFlag, host, port string, dryRun bool) error {
	msg, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		return fmt.Errorf("parse failed: %w", err)
	}

	sender := fromFlag
	if sender == "" {
		addrs, err := msg.Header.AddressList("From")
		if err != nil || len(addrs) == 0 {
			return fmt.Errorf("could not determine sender; use --from")
		}
		sender = addrs[0].Address
	}

	var recipients []string
	if toFlag != "" {
		for a := range strings.SplitSeq(toFlag, ",") {
			if t := strings.TrimSpace(a); t != "" {
				recipients = append(recipients, t)
			}
		}
	} else {
		for _, hdr := range []string{"To", "Cc"} {
			addrs, err := msg.Header.AddressList(hdr)
			if err != nil {
				continue
			}
			for _, a := range addrs {
				recipients = append(recipients, a.Address)
			}
		}
		if len(recipients) == 0 {
			return fmt.Errorf("could not determine recipients; use --to")
		}
	}

	fmt.Printf("  From:    %s\n", sender)
	fmt.Printf("  To:      %s\n", strings.Join(recipients, ", "))
	fmt.Printf("  Subject: %s\n", msg.Header.Get("Subject"))
	fmt.Printf("  Size:    %d bytes\n", len(raw))

	if dryRun {
		fmt.Println("  [dry-run, not sending]")
		return nil
	}

	addr := net.JoinHostPort(host, port)
	c, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	defer c.Close()

	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{InsecureSkipVerify: true}); err != nil { //nolint:gosec
			return fmt.Errorf("starttls: %w", err)
		}
	}

	if err := c.Mail(sender); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}
	for _, r := range recipients {
		if err := c.Rcpt(r); err != nil {
			return fmt.Errorf("RCPT TO %s: %w", r, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close data: %w", err)
	}
	if err := c.Quit(); err != nil {
		return fmt.Errorf("quit: %w", err)
	}

	fmt.Println("  OK")
	return nil
}

// isMboxContent reports whether data starts with the mbox "From " delimiter.
func isMboxContent(data []byte) bool {
	return bytes.HasPrefix(data, []byte("From "))
}

// splitMbox splits mbox formatted data into individual RFC 2822 messages.
// It handles the mboxrd format where ">From " at the start of a line is
// an escaped "From ".
func splitMbox(data []byte) [][]byte {
	lines := bytes.Split(data, []byte("\n"))

	var msgs [][]byte
	var start int
	for i, line := range lines {
		if bytes.HasPrefix(line, []byte("From ")) && i > start {
			msgs = append(msgs, extractMessage(lines[start+1:i]))
			start = i
		}
	}
	if start < len(lines) {
		msgs = append(msgs, extractMessage(lines[start+1:]))
	}

	// Remove trailing empty messages.
	for len(msgs) > 0 && len(bytes.TrimSpace(msgs[len(msgs)-1])) == 0 {
		msgs = msgs[:len(msgs)-1]
	}

	return msgs
}

// extractMessage joins lines and unescapes mboxrd ">From " -> "From ".
func extractMessage(lines [][]byte) []byte {
	var buf bytes.Buffer
	for _, line := range lines {
		if bytes.HasPrefix(line, []byte(">From ")) {
			buf.Write(line[1:])
		} else {
			buf.Write(line)
		}
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}
