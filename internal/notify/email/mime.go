package email

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"mime"
	"strings"
	"time"

	"github.com/azazo1/rainmail/internal/notify"
)

// buildMessage 构造 RFC 5322 邮件: 有 HTML 正文时使用 multipart/alternative.
func (n *Notifier) buildMessage(msg notify.Message) ([]byte, error) {
	subject := strings.TrimSpace(n.cfg.SubjectPrefix + " " + msg.Title)
	boundary := "rainmail-" + randomHex(16)

	var buf bytes.Buffer
	writeHeader(&buf, "From", formatAddress(n.cfg.FromName, n.cfg.From))
	writeHeader(&buf, "To", strings.Join(formatAddressList(n.cfg.To), ", "))
	writeHeader(&buf, "Subject", encodeHeader(subject))
	writeHeader(&buf, "Date", time.Now().Format(time.RFC1123Z))
	writeHeader(&buf, "Message-ID", fmt.Sprintf("<%s@rainmail>", randomHex(12)))
	writeHeader(&buf, "MIME-Version", "1.0")
	writeHeader(&buf, "X-Mailer", "rainmail")

	if strings.TrimSpace(msg.HTML) != "" {
		writeHeader(&buf, "Content-Type", fmt.Sprintf("multipart/alternative; boundary=%q", boundary))
		buf.WriteString("\r\n")
		writeBodyPart(&buf, boundary, "text/plain; charset=UTF-8", msg.Text)
		writeBodyPart(&buf, boundary, "text/html; charset=UTF-8", msg.HTML)
		buf.WriteString("--" + boundary + "--\r\n")
	} else {
		writeHeader(&buf, "Content-Type", "text/plain; charset=UTF-8")
		writeHeader(&buf, "Content-Transfer-Encoding", "base64")
		buf.WriteString("\r\n")
		writeBase64(&buf, msg.Text)
	}

	return buf.Bytes(), nil
}

func writeHeader(buf *bytes.Buffer, key, value string) {
	buf.WriteString(key)
	buf.WriteString(": ")
	buf.WriteString(value)
	buf.WriteString("\r\n")
}

func writeBodyPart(buf *bytes.Buffer, boundary, contentType, body string) {
	buf.WriteString("--" + boundary + "\r\n")
	writeHeader(buf, "Content-Type", contentType)
	writeHeader(buf, "Content-Transfer-Encoding", "base64")
	buf.WriteString("\r\n")
	writeBase64(buf, body)
}

// writeBase64 按 76 列折行输出 base64 正文, 兼容中文与各类邮件客户端.
func writeBase64(buf *bytes.Buffer, text string) {
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	for len(encoded) > 76 {
		buf.WriteString(encoded[:76])
		buf.WriteString("\r\n")
		encoded = encoded[76:]
	}
	if encoded != "" {
		buf.WriteString(encoded)
		buf.WriteString("\r\n")
	}
}

// encodeHeader 对非 ASCII 头部按 RFC 2047 编码.
func encodeHeader(value string) string {
	for _, r := range value {
		if r > 127 {
			return mime.QEncoding.Encode("UTF-8", value)
		}
	}
	return value
}

func formatAddress(name, address string) string {
	if strings.TrimSpace(name) == "" {
		return address
	}
	return fmt.Sprintf("%s <%s>", encodeHeader(name), address)
}

func formatAddressList(addresses []string) []string {
	out := make([]string, 0, len(addresses))
	for _, address := range addresses {
		out = append(out, strings.TrimSpace(address))
	}
	return out
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
