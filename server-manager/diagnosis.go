package main

import (
	"net"
	"net/url"
	"strings"
	"time"
)

type portProbe struct {
	address, result string
	checked         time.Time
	waiting         bool
}

func probeAddress(raw string) string {
	if !validBrowserURL(raw) {
		return ""
	}
	u, _ := url.Parse(raw)
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	return net.JoinHostPort(u.Hostname(), port)
}

func probePort(address string) string {
	conn, err := net.DialTimeout("tcp", address, 600*time.Millisecond)
	if err != nil {
		return "연결할 수 없음"
	}
	conn.Close()
	return "연결 가능"
}

func portConflict(log string) bool {
	log = strings.ToLower(log)
	return strings.Contains(log, "address already in use") || strings.Contains(log, "eaddrinuse") ||
		(strings.Contains(log, "port ") && strings.Contains(log, " is already in use"))
}

func (m *manager) portStatus(raw string) string {
	address := probeAddress(raw)
	if address == "" {
		return "포트 · 브라우저 주소를 설정하세요"
	}
	if !m.probe.waiting && (m.probe.address != address || time.Since(m.probe.checked) >= 2*time.Second) {
		m.probe.waiting = true
		go func() {
			result := probePort(address)
			m.App.QueueUpdateDraw(func() {
				m.probe = portProbe{address: address, result: result, checked: time.Now()}
				m.refreshDetails(m.list.GetCurrentItem())
			})
		}()
	}
	if m.probe.address != address {
		return "포트 · " + address + " 확인 중"
	}
	return "포트 · " + address + " " + m.probe.result
}
