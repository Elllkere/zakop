package importer

import (
	"encoding/base64"
	"testing"
)

func TestParseVLESSRealityLink(t *testing.T) {
	node, err := ParseLink("vless://a3482e88-686a-4a58-8126-99c9df64b060@example.com:443?security=reality&sni=www.example.com&flow=xtls-rprx-vision&pbk=public-key&sid=0123&fp=edge&type=tcp&packetEncoding=xudp#My%20VLESS")
	if err != nil {
		t.Fatal(err)
	}
	out := node.Outbound
	if out.Type != "vless" || out.Label != "My VLESS" || out.Server != "example.com" || out.Port != 443 || out.UUID == "" {
		t.Fatalf("unexpected vless: %+v", out)
	}
	if !out.TLS || !out.Reality || out.RealityPublicKey != "public-key" || out.RealityShortID != "0123" || out.UTLSFingerprint != "edge" || out.PacketEncoding != "xudp" {
		t.Fatalf("missing reality fields: %+v", out)
	}
}

func TestParseHysteria2Link(t *testing.T) {
	node, err := ParseLink("hysteria2://secret@example.com:443?sni=example.com&insecure=1&obfs=salamander&obfs-password=obfs#HY2")
	if err != nil {
		t.Fatal(err)
	}
	out := node.Outbound
	if out.Type != "hysteria2" || out.Password != "secret" || out.ServerName != "example.com" || !out.Insecure || out.HysteriaObfsPassword != "obfs" {
		t.Fatalf("unexpected hysteria2: %+v", out)
	}
}

func TestParseShadowsocksSIP002Link(t *testing.T) {
	user := base64.RawURLEncoding.EncodeToString([]byte("2022-blake3-aes-128-gcm:ss-secret"))
	node, err := ParseLink("ss://" + user + "@example.com:8388#SS")
	if err != nil {
		t.Fatal(err)
	}
	out := node.Outbound
	if out.Type != "shadowsocks" || out.Method != "2022-blake3-aes-128-gcm" || out.Password != "ss-secret" || out.Server != "example.com" || out.Port != 8388 {
		t.Fatalf("unexpected shadowsocks: %+v", out)
	}
}

func TestParseTrojanLink(t *testing.T) {
	node, err := ParseLink("trojan://trojan-secret@example.com:443?sni=example.com&type=ws&host=front.example.com&path=%2Fws#Trojan")
	if err != nil {
		t.Fatal(err)
	}
	out := node.Outbound
	if out.Type != "trojan" || out.Password != "trojan-secret" || !out.TLS || out.Transport != "ws" || out.WSHost != "front.example.com" || out.WSPath != "/ws" {
		t.Fatalf("unexpected trojan: %+v", out)
	}
}

func TestParseLinkPreservesUnicodeFlagLabel(t *testing.T) {
	node, err := ParseLink("vless://a3482e88-686a-4a58-8126-99c9df64b060@example.com:443#%F0%9F%87%AB%F0%9F%87%AE%D0%A4%D0%B8%D0%BD%D0%BB%D1%8F%D0%BD%D0%B4%D0%B8%D1%8F")
	if err != nil {
		t.Fatal(err)
	}
	if node.Outbound.Label != "🇫🇮Финляндия" {
		t.Fatalf("got label %q, want Unicode flag label", node.Outbound.Label)
	}
}

func TestParseBase64Subscription(t *testing.T) {
	body := "trojan://secret@example.com:443#Trojan\nvless://a3482e88-686a-4a58-8126-99c9df64b060@example.com:443#VLESS"
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	nodes, err := ParseLinks(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].Outbound.Type != "trojan" || nodes[1].Outbound.Type != "vless" {
		t.Fatalf("unexpected nodes: %+v", nodes)
	}
}
