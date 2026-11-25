package main

import (
	"net"
	"testing"
)

func TestLocalPortAndConflictDiagnosis(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if got := probeAddress("http://" + address + "/path?query"); got != address {
		t.Fatalf("got %q for loopback address %q", got, address)
	}
	if probeAddress("https://example.com") != "" || probeAddress("https://localhost") != "localhost:443" {
		t.Fatal("unexpected host or default port")
	}
	if got := probePort(address); got != "연결 가능" {
		t.Fatalf("listening port: %q", got)
	}
	listener.Close()
	if got := probePort(address); got != "연결할 수 없음" {
		t.Fatalf("closed port: %q", got)
	}
	if !portConflict("Error: EADDRINUSE") || !portConflict("bind: Address already in use") || portConflict("listening on port 3000") {
		t.Fatal("unexpected conflict result")
	}
}
