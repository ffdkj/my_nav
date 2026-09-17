package favicon

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// safeControl 在**实际拨号那一刻**校验目标 IP。
//
// 为什么不用"先解析域名再检查"：那是 TOCTOU —— 校验与拨号之间 DNS 可以变，
// 也就是 DNS rebinding。net.Dialer.Control 拿到的是即将连接的地址，堵住了这个缝。
func safeControl(_ string, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("favicon: bad dial address %q: %w", address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("favicon: unresolved dial address %q", address)
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return fmt.Errorf("favicon: blocked address %s", address)
	}
	return nil
}

// newClient 构造抓取用的 HTTP 客户端：
// 限时、限跳数、限大小（大小在读取处限制），并在拨号层做 SSRF 防护。
func newClient(timeout time.Duration, allowPrivate bool) *http.Client {
	dialer := &net.Dialer{Timeout: 3 * time.Second}
	if !allowPrivate {
		dialer.Control = safeControl
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext:           dialer.DialContext,
			TLSHandshakeTimeout:   3 * time.Second,
			ResponseHeaderTimeout: 3 * time.Second,
			MaxIdleConns:          8,
			MaxIdleConnsPerHost:   2,
			ForceAttemptHTTP2:     true,
		},
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("favicon: too many redirects")
			}
			// 跳转目标会在拨号时被 safeControl 重新校验，这里只限跳数
			return nil
		},
	}
}
