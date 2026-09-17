package favicon

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"
)

// proxyConfig 在**构造时**解析一次代理设置。
//
// 为什么不用 http.ProxyFromEnvironment：它把结果缓存在进程级
// （httpproxy 的 envProxyOnce），一旦被调用过，之后再改环境变量都不生效 ——
// 这在生产无妨（env 启动即定），但会让测试互相污染，且行为难以推理。
// 这里显式解析，语义清楚，测试里 setenv 后新建实例即可生效。
func proxyConfig() (*url.URL, []string) {
	raw := ""
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			raw = v
			break
		}
	}
	var noProxy []string
	for _, key := range []string{"NO_PROXY", "no_proxy"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			for _, part := range strings.Split(v, ",") {
				if p := strings.TrimSpace(part); p != "" {
					noProxy = append(noProxy, strings.ToLower(p))
				}
			}
		}
	}
	if raw == "" {
		return nil, noProxy
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, noProxy
	}
	return u, noProxy
}

func hostMatchesNoProxy(host string, noProxy []string) bool {
	host = strings.ToLower(host)
	for _, entry := range noProxy {
		if entry == "*" || host == entry || strings.HasSuffix(host, "."+strings.TrimPrefix(entry, ".")) {
			return true
		}
	}
	return false
}

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
	proxyURL, noProxy := proxyConfig()

	dialer := &net.Dialer{Timeout: 3 * time.Second}
	// 有代理时不按目标 IP 校验：此时拨号对象是**代理自身**（常常就在内网/回环），
	// 按私网拦截会把代理也一起拦掉，等于代理形同虚设。
	// 换句话说：一旦运维显式配了代理，出网策略就由代理负责。
	if !allowPrivate && proxyURL == nil {
		dialer.Control = safeControl
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			// 支持 HTTP(S)_PROXY / NO_PROXY。
			// 自托管场景里这很关键：墙内主机（或必须走代理才能出网的环境）
			// 没这一步就永远抓不到图标/壁纸，整条抓取链形同虚设。
			Proxy: func(req *http.Request) (*url.URL, error) {
				if proxyURL == nil || hostMatchesNoProxy(req.URL.Hostname(), noProxy) {
					return nil, nil
				}
				return proxyURL, nil
			},
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
