// Package email 通过 SMTP 发送提醒邮件.
//
// 支持三种连接方式: tls(隐式 TLS, 常用 465), starttls(常用 587), none(明文, 常用 25).
// 认证优先使用 PLAIN, 服务器不支持时退回 LOGIN.
package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/azazo1/rainmail/internal/notify"
)

// 加密方式取值.
const (
	EncryptionTLS      = "tls"
	EncryptionStartTLS = "starttls"
	EncryptionNone     = "none"
)

// Config 是 SMTP 发送配置.
type Config struct {
	Host               string
	Port               int
	Encryption         string
	Username           string
	Password           string
	From               string
	FromName           string
	To                 []string
	SubjectPrefix      string
	InsecureSkipVerify bool
	Timeout            time.Duration
}

// Notifier 是邮件提醒通道.
type Notifier struct {
	cfg    Config
	logger *slog.Logger
}

// New 校验并构造邮件通道.
func New(cfg Config, logger *slog.Logger) (*Notifier, error) {
	if strings.TrimSpace(cfg.Host) == "" {
		return nil, errors.New("邮件通道缺少 host")
	}
	if cfg.Port == 0 {
		cfg.Port = 465
	}
	if cfg.Encryption == "" {
		cfg.Encryption = EncryptionForPort(cfg.Port)
	}
	switch cfg.Encryption {
	case EncryptionTLS, EncryptionStartTLS, EncryptionNone:
	default:
		return nil, fmt.Errorf("未知加密方式 %q, 可选 tls / starttls / none", cfg.Encryption)
	}
	if cfg.From == "" {
		cfg.From = cfg.Username
	}
	if cfg.From == "" {
		return nil, errors.New("邮件通道缺少发件地址 from")
	}
	if len(cfg.To) == 0 {
		return nil, errors.New("邮件通道缺少收件地址 to")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 20 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Notifier{cfg: cfg, logger: logger}, nil
}

// EncryptionForPort 依据端口推断加密方式.
func EncryptionForPort(port int) string {
	switch port {
	case 465:
		return EncryptionTLS
	case 587, 25:
		return EncryptionStartTLS
	default:
		return EncryptionTLS
	}
}

// Name 返回通道名.
func (n *Notifier) Name() string { return "email" }

// Send 渲染并投递一封提醒邮件.
func (n *Notifier) Send(ctx context.Context, msg notify.Message) error {
	data, err := n.buildMessage(msg)
	if err != nil {
		return err
	}

	start := time.Now()
	if err := n.deliver(ctx, data); err != nil {
		return err
	}
	n.logger.Debug("邮件已投递",
		"host", n.cfg.Host,
		"port", n.cfg.Port,
		"encryption", n.cfg.Encryption,
		"recipients", len(n.cfg.To),
		"bytes", len(data),
		"elapsed", time.Since(start).Round(time.Millisecond))
	return nil
}

func (n *Notifier) deliver(ctx context.Context, data []byte) error {
	client, err := n.dial(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	if err := client.Mail(n.cfg.From); err != nil {
		return fmt.Errorf("SMTP MAIL FROM %s 失败: %w", n.cfg.From, err)
	}
	for _, rcpt := range n.cfg.To {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("SMTP RCPT TO %s 失败: %w", rcpt, err)
		}
	}

	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA 失败: %w", err)
	}
	if _, err := writer.Write(data); err != nil {
		_ = writer.Close()
		return fmt.Errorf("写入邮件正文失败: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("提交邮件正文失败: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("SMTP QUIT 失败: %w", err)
	}
	return nil
}

func (n *Notifier) dial(ctx context.Context) (*smtp.Client, error) {
	address := net.JoinHostPort(n.cfg.Host, strconv.Itoa(n.cfg.Port))
	dialer := &net.Dialer{Timeout: n.cfg.Timeout}

	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("连接 SMTP 服务器 %s 失败: %w", address, err)
	}
	// net/smtp 不感知 context, 这里用连接级超时兜住整体耗时.
	deadline := time.Now().Add(n.cfg.Timeout)
	if err := conn.SetDeadline(deadline); err != nil {
		n.logger.Debug("设置 SMTP 连接超时失败", "error", err)
	}

	tlsConfig := &tls.Config{
		ServerName:         n.cfg.Host,
		InsecureSkipVerify: n.cfg.InsecureSkipVerify,
		MinVersion:         tls.VersionTLS12,
	}

	if n.cfg.Encryption == EncryptionTLS {
		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("与 %s 建立 TLS 连接失败: %w", address, err)
		}
		conn = tlsConn
	}

	client, err := smtp.NewClient(conn, n.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("初始化 SMTP 会话失败: %w", err)
	}

	if n.cfg.Encryption == EncryptionStartTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			_ = client.Close()
			return nil, fmt.Errorf("SMTP 服务器 %s 不支持 STARTTLS, 可改用 encryption = \"tls\" 或 \"none\"", address)
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("STARTTLS 握手失败: %w", err)
		}
	}

	if n.cfg.Username != "" {
		if err := client.Auth(n.chooseAuth(client)); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("SMTP 认证失败(请检查用户名与授权码): %w", err)
		}
	}

	return client, nil
}

func (n *Notifier) chooseAuth(client *smtp.Client) smtp.Auth {
	if ok, mechanisms := client.Extension("AUTH"); ok {
		upper := strings.ToUpper(mechanisms)
		if !strings.Contains(upper, "PLAIN") && strings.Contains(upper, "LOGIN") {
			return &loginAuth{username: n.cfg.Username, password: n.cfg.Password}
		}
	}
	return smtp.PlainAuth("", n.cfg.Username, n.cfg.Password, n.cfg.Host)
}

// loginAuth 实现 AUTH LOGIN, 兼容只支持 LOGIN 的服务器.
type loginAuth struct {
	username string
	password string
}

// Start 发起 LOGIN 认证.
func (a *loginAuth) Start(*smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", nil, nil
}

// Next 依次回应服务端的用户名与口令挑战.
func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	prompt := strings.ToLower(string(fromServer))
	switch {
	case strings.Contains(prompt, "username"):
		return []byte(a.username), nil
	case strings.Contains(prompt, "password"):
		return []byte(a.password), nil
	default:
		return nil, fmt.Errorf("SMTP 认证出现未预期的挑战: %s", strings.TrimSpace(string(fromServer)))
	}
}
