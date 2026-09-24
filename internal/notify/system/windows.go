//go:build windows

package system

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"os/exec"
	"strings"
	"unicode/utf16"
)

// available 判断是否存在 PowerShell.
func available() bool {
	_, err := findPowerShell()
	return err == nil
}

// notifySystem 通过 WinRT 的 Toast 通知提醒用户.
func notifySystem(ctx context.Context, title, body string, sound bool) error {
	shell, err := findPowerShell()
	if err != nil {
		return err
	}

	script := buildToastScript(title, body, sound)
	encoded := encodeUTF16LEBase64(script)

	output, err := exec.CommandContext(ctx, shell,
		"-NoProfile", "-NonInteractive", "-EncodedCommand", encoded).CombinedOutput()
	if err != nil {
		return fmt.Errorf("调用 PowerShell 弹出通知失败: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func findPowerShell() (string, error) {
	for _, name := range []string{"powershell", "powershell.exe", "pwsh"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("未找到 PowerShell, 无法发送 Windows 通知")
}

// buildToastScript 生成 Toast 脚本.
//
// 通知的 XML 以 base64 传递并在脚本内解码, 避免文案中的引号或换行破坏脚本结构.
func buildToastScript(title, body string, sound bool) string {
	audio := ""
	if sound {
		audio = `<audio src="ms-winsoundevent:Notification.Default" />`
	}

	toast := "<toast><visual><binding template=\"ToastGeneric\"><text>" +
		escapeXML(title) + "</text><text>" + escapeXML(body) + "</text></binding></visual>" +
		audio + "</toast>"
	payload := base64.StdEncoding.EncodeToString([]byte(toast))

	return strings.Join([]string{
		"$ErrorActionPreference = 'Stop'",
		"[void][Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime]",
		"[void][Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime]",
		"$toastXml = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" + payload + "'))",
		"$document = New-Object Windows.Data.Xml.Dom.XmlDocument",
		"$document.LoadXml($toastXml)",
		"$toast = New-Object Windows.UI.Notifications.ToastNotification $document",
		"[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('rainmail').Show($toast)",
	}, "\r\n")
}

func escapeXML(s string) string {
	var buf strings.Builder
	if err := xml.EscapeText(&buf, []byte(s)); err != nil {
		return s
	}
	return buf.String()
}

// encodeUTF16LEBase64 生成 -EncodedCommand 需要的 UTF-16LE base64 文本.
func encodeUTF16LEBase64(script string) string {
	units := utf16.Encode([]rune(script))
	raw := make([]byte, len(units)*2)
	for i, unit := range units {
		binary.LittleEndian.PutUint16(raw[i*2:], unit)
	}
	return base64.StdEncoding.EncodeToString(raw)
}
