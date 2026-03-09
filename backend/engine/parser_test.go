package engine

import (
	"encoding/base64"
	"strings"
	"testing"

	"meta-link-pro/backend/models"
)

func TestParseVLESSReality(t *testing.T) {
	link := "vless://123e4567-e89b-12d3-a456-426614174000@example.com:443?security=reality&pbk=abc123&sid=ff&sni=google.com&type=grpc&serviceName=mygrpc#NodeA"
	node, issue := ParseLink(link)
	if issue != nil {
		t.Fatalf("unexpected issue: %+v", issue)
	}
	if node.Protocol != models.ProtocolVLESS {
		t.Fatalf("unexpected protocol: %s", node.Protocol)
	}
	if node.PublicKey != "abc123" || node.ShortID != "ff" || node.ServiceName != "mygrpc" {
		t.Fatalf("unexpected vless fields: %+v", node)
	}
}

func TestParseTUICMissingToken(t *testing.T) {
	link := "tuic://123e4567-e89b-12d3-a456-426614174000@example.com:443?congestion_control=bbr#TUIC"
	_, issue := ParseLink(link)
	if issue == nil {
		t.Fatal("expected token missing issue")
	}
	if !strings.Contains(issue.Message, "Token缺失") {
		t.Fatalf("unexpected issue: %+v", issue)
	}
}

func TestParseTUICUserInfo(t *testing.T) {
	link := "tuic://123e4567-e89b-12d3-a456-426614174000:mytoken@example.com:443?congestion-control=cubic&udp-relay-mode=quic&alpn=h3,hq#TUIC-User"
	node, issue := ParseLink(link)
	if issue != nil {
		t.Fatalf("unexpected issue: %+v", issue)
	}
	if node.Protocol != models.ProtocolTUIC {
		t.Fatalf("unexpected protocol: %s", node.Protocol)
	}
	if node.Token != "mytoken" || node.CongestionControl != "cubic" || node.UDPRelayMode != "quic" {
		t.Fatalf("unexpected tuic fields: %+v", node)
	}
}

func TestParseSS2022WithPlugin(t *testing.T) {
	link := "ss://2022-blake3-aes-128-gcm:pass123@example.com:443?plugin=v2ray-plugin%3Bmode%3Dwebsocket%3Bhost%3Dcdn.example.com#SS2022"
	node, issue := ParseLink(link)
	if issue != nil {
		t.Fatalf("unexpected issue: %+v", issue)
	}
	if node.Protocol != models.ProtocolSS {
		t.Fatalf("unexpected protocol: %s", node.Protocol)
	}
	if node.Cipher != "2022-blake3-aes-128-gcm" || node.Password != "pass123" {
		t.Fatalf("unexpected ss credentials: %+v", node)
	}
	if node.Plugin != "v2ray-plugin" || !strings.Contains(node.PluginOpts, "mode=websocket") {
		t.Fatalf("unexpected plugin fields: %+v", node)
	}
}

func TestParseClashYAMLProxies(t *testing.T) {
	content := `proxies:
  - name: VLESS-A
    type: vless
    server: vless.example.com
    port: 443
    uuid: 123e4567-e89b-12d3-a456-426614174000
    tls: true
    network: grpc
    grpc-opts:
      grpc-service-name: meta-link
  - { name: TUIC-B, type: tuic, server: tuic.example.com, port: 443, uuid: 123e4567-e89b-12d3-a456-426614174001, token: abc123, congestion-controller: bbr, udp-relay-mode: native }
`
	report := ParseInput(content)
	if len(report.Errors) != 0 {
		t.Fatalf("unexpected errors: %+v", report.Errors)
	}
	if len(report.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(report.Nodes))
	}
	if report.Nodes[0].Protocol != models.ProtocolVLESS || report.Nodes[1].Protocol != models.ProtocolTUIC {
		t.Fatalf("unexpected protocols: %+v", report.Nodes)
	}
}

func TestParseClashYAMLWithAnchorsAndMerge(t *testing.T) {
	content := `defaults: &base
  type: vless
  server: base.example.com
  port: 443
  uuid: 123e4567-e89b-12d3-a456-426614174000
  tls: true
  network: ws
  ws-opts:
    path: /ws
    headers:
      Host: cdn.example.com

proxies:
  - <<: *base
    name: VLESS-ANCHOR-1
  - <<: *base
    name: VLESS-ANCHOR-2
    server: second.example.com
`
	report := ParseInput(content)
	if len(report.Errors) != 0 {
		t.Fatalf("unexpected errors: %+v", report.Errors)
	}
	if len(report.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(report.Nodes))
	}
	if report.Nodes[0].Protocol != models.ProtocolVLESS {
		t.Fatalf("unexpected protocol: %s", report.Nodes[0].Protocol)
	}
	if report.Nodes[0].Path != "/ws" || report.Nodes[0].Host != "cdn.example.com" {
		t.Fatalf("anchor fields not merged: %+v", report.Nodes[0])
	}
	if report.Nodes[1].Server != "second.example.com" {
		t.Fatalf("override not applied: %+v", report.Nodes[1])
	}
}

func TestShouldFetchSubscriptionForPlainHTTPLink(t *testing.T) {
	if !shouldFetchSubscription("https://example.com/plain-sub", 1, false) {
		t.Fatal("expected plain single http link to be treated as subscription")
	}
	if shouldFetchSubscription("https://example.com/readme", 2, false) {
		t.Fatal("expected multi-link plain http to not be forced as subscription")
	}
}

func TestParseInputBase64PlainSubscriptionBody(t *testing.T) {
	raw := strings.Join([]string{
		"vless://123e4567-e89b-12d3-a456-426614174000@example.com:443?type=tcp#VLESS-1",
		"trojan://passwd@example.org:443#TR-1",
	}, "\n")
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))

	report := ParseInput(encoded)
	if len(report.Errors) != 0 {
		t.Fatalf("unexpected errors: %+v", report.Errors)
	}
	if len(report.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(report.Nodes))
	}
}
