package dnsproxy

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/elllkere/neto/internal/config"
	"github.com/elllkere/neto/internal/ruleengine"
)

type Proxy struct {
	Listen              string
	FakeUpstream        string
	RealDirectUpstream  string
	RealProxyUpstream   string
	RealDNSMode         string
	RoutingMode         string
	ClientPolicies      map[string]string
	LANSubnets          []*net.IPNet
	LocalIPs            map[string]struct{}
	Rules               []config.Rule
	FilterAAAAForFakeIP bool
	Timeout             time.Duration
	fakeForwardSlots    chan struct{}
	realForwardSlots    chan struct{}
}

type Query struct {
	Name        string
	Type        uint16
	QuestionEnd int
}

const (
	qTypeA                 uint16 = 1
	qTypeAAAA              uint16 = 28
	rrTypeOPT              uint16 = 41
	ednsClientSubnetOption uint16 = 8
	// Keep the combined ceiling below dnsmasq's default dns-forward-max of
	// 150. Separate pools prevent a failed real-DNS path from starving FakeIP.
	maxFakeDNSForwards = 32
	maxRealDNSForwards = 96
)

func New(cfg config.Config) Proxy {
	clientPolicies := map[string]string{}
	for _, client := range cfg.Clients {
		clientPolicies[client.IP] = client.Policy
	}
	lanSubnets := make([]*net.IPNet, 0, len(cfg.Main.LANSubnets))
	for _, raw := range cfg.Main.LANSubnets {
		_, subnet, err := net.ParseCIDR(raw)
		if err == nil && subnet != nil {
			lanSubnets = append(lanSubnets, subnet)
		}
	}
	return Proxy{
		Listen:              cfg.Main.DNSListen,
		FakeUpstream:        cfg.Main.SingBoxDNSFakeIPAddr(),
		RealDirectUpstream:  cfg.Main.SingBoxDNSRealDirectAddr(),
		RealProxyUpstream:   cfg.Main.SingBoxDNSRealProxyAddr(),
		RealDNSMode:         cfg.Main.RealDNSMode,
		RoutingMode:         cfg.Main.RoutingMode,
		ClientPolicies:      clientPolicies,
		LANSubnets:          lanSubnets,
		LocalIPs:            localInterfaceIPs(),
		Rules:               append([]config.Rule(nil), cfg.EffectiveRules()...),
		FilterAAAAForFakeIP: cfg.Main.FilterAAAAForFakeIP,
		Timeout:             5 * time.Second,
		fakeForwardSlots:    make(chan struct{}, maxFakeDNSForwards),
		realForwardSlots:    make(chan struct{}, maxRealDNSForwards),
	}
}

func (p Proxy) Run(ctx context.Context) error {
	for label, upstream := range map[string]string{
		"fakeip":      p.FakeUpstream,
		"real-direct": p.RealDirectUpstream,
		"real-proxy":  p.RealProxyUpstream,
	} {
		if upstream == p.Listen {
			return fmt.Errorf("%s DNS upstream points back to neto DNS listener", label)
		}
	}

	lc := net.ListenConfig{}
	udpConn, err := lc.ListenPacket(ctx, "udp", p.Listen)
	if err != nil {
		return fmt.Errorf("listen udp %s: %w", p.Listen, err)
	}
	defer udpConn.Close()

	tcpLn, err := lc.Listen(ctx, "tcp", p.Listen)
	if err != nil {
		return fmt.Errorf("listen tcp %s: %w", p.Listen, err)
	}
	defer tcpLn.Close()

	errCh := make(chan error, 2)
	go p.serveUDP(ctx, udpConn, errCh)
	go p.serveTCP(ctx, tcpLn, errCh)

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		if err == nil {
			return nil
		}
		return err
	}
}

func (p Proxy) serveUDP(ctx context.Context, conn net.PacketConn, errCh chan<- error) {
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	for {
		buf := make([]byte, 4096)
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				errCh <- nil
				return
			}
			errCh <- err
			return
		}
		msg := append([]byte(nil), buf[:n]...)
		go func() {
			resp, err := p.handleUDP(ctx, msg, p.clientIP(msg, sourceIP(addr)))
			if err == nil && len(resp) > 0 {
				_, _ = conn.WriteTo(resp, addr)
			}
		}()
	}
}

func (p Proxy) serveTCP(ctx context.Context, ln net.Listener, errCh chan<- error) {
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				errCh <- nil
				return
			}
			errCh <- err
			return
		}
		go p.handleTCP(ctx, conn)
	}
}

func (p Proxy) handleTCP(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(p.Timeout))

	msg, err := readTCPDNS(conn)
	if err != nil {
		return
	}
	resp, err := p.handleTCPQuery(ctx, msg, p.clientIP(msg, sourceIP(conn.RemoteAddr())))
	if err != nil || len(resp) == 0 {
		return
	}
	_ = writeTCPDNS(conn, resp)
}

func (p Proxy) handleUDP(ctx context.Context, msg []byte, clientIP string) ([]byte, error) {
	if resp, ok := p.localResponse(msg, clientIP); ok {
		return resp, nil
	}
	upstream := p.upstreamFor(msg, clientIP)
	release, ok := p.reserveForward(upstream)
	if !ok {
		return servfailResponse(msg), nil
	}
	defer release()

	resp, err := p.forwardUDP(ctx, stripClientSubnetOption(msg), upstream)
	if err != nil {
		return servfailResponse(msg), nil
	}
	return resp, nil
}

func (p Proxy) handleTCPQuery(ctx context.Context, msg []byte, clientIP string) ([]byte, error) {
	if resp, ok := p.localResponse(msg, clientIP); ok {
		return resp, nil
	}
	upstream := p.upstreamFor(msg, clientIP)
	release, ok := p.reserveForward(upstream)
	if !ok {
		return servfailResponse(msg), nil
	}
	defer release()

	resp, err := p.forwardTCP(ctx, stripClientSubnetOption(msg), upstream)
	if err != nil {
		return servfailResponse(msg), nil
	}
	return resp, nil
}

func (p Proxy) reserveForward(upstream string) (func(), bool) {
	slots := p.realForwardSlots
	if upstream == p.FakeUpstream {
		slots = p.fakeForwardSlots
	}
	if slots == nil {
		return func() {}, true
	}
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, true
	default:
		return nil, false
	}
}

func (p Proxy) forwardUDP(ctx context.Context, msg []byte, upstream string) ([]byte, error) {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "udp", upstream)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(p.Timeout))

	if _, err := conn.Write(msg); err != nil {
		return nil, err
	}
	resp := make([]byte, 4096)
	n, err := conn.Read(resp)
	if err != nil {
		return nil, err
	}
	return resp[:n], nil
}

func (p Proxy) forwardTCP(ctx context.Context, msg []byte, upstream string) ([]byte, error) {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", upstream)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(p.Timeout))

	if err := writeTCPDNS(conn, msg); err != nil {
		return nil, err
	}
	return readTCPDNS(conn)
}

func (p Proxy) upstreamFor(msg []byte, clientIP string) string {
	if p.useFakeUpstream(msg, clientIP) {
		return p.FakeUpstream
	}
	if p.RealDNSMode == "proxy" {
		return p.RealProxyUpstream
	}
	return p.RealDirectUpstream
}

func (p Proxy) useFakeUpstream(msg []byte, clientIP string) bool {
	query, ok := ParseQuery(msg)
	if !ok {
		return false
	}
	decision := p.domainDecision(query.Name, clientIP)
	return decision.Action == "proxy" && decision.DNSMode == "fakeip"
}

func (p Proxy) localResponse(msg []byte, clientIP string) ([]byte, bool) {
	query, ok := ParseQuery(msg)
	if !ok {
		return nil, false
	}
	decision := p.domainDecision(query.Name, clientIP)
	if decision.Action == "block" {
		return nxdomainResponse(msg, query.QuestionEnd), true
	}
	if p.FilterAAAAForFakeIP && query.Type == qTypeAAAA && decision.Action == "proxy" && decision.DNSMode == "fakeip" {
		return nodataResponse(msg, query.QuestionEnd), true
	}
	return nil, false
}

func (p Proxy) domainDecision(name string, clientIP string) ruleengine.Decision {
	if !p.isLANClient(clientIP) {
		return ruleengine.Decision{Action: "direct", DNSMode: "real_ip"}
	}
	policy := p.clientPolicy(clientIP)
	if policy == "direct" {
		return ruleengine.Decision{Action: "direct", DNSMode: "real_ip"}
	}
	if p.RoutingMode == "global" {
		return ruleengine.Decision{Action: "direct", DNSMode: "real_ip"}
	}
	return ruleengine.DomainDecision(p.Rules, name)
}

func (p Proxy) isLANClient(clientIP string) bool {
	if len(p.LANSubnets) == 0 {
		return true
	}
	ip := net.ParseIP(strings.TrimSpace(clientIP))
	if ip == nil || ip.To4() == nil || ip.IsLoopback() {
		return false
	}
	canonical := ip.String()
	if _, ok := p.LocalIPs[canonical]; ok {
		return false
	}
	for _, subnet := range p.LANSubnets {
		if subnet != nil && subnet.Contains(ip) {
			return true
		}
	}
	return false
}

func localInterfaceIPs() map[string]struct{} {
	result := map[string]struct{}{}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return result
	}
	for _, addr := range addrs {
		raw := addr.String()
		if host, _, err := net.ParseCIDR(raw); err == nil && host != nil {
			result[host.String()] = struct{}{}
			continue
		}
		if host := net.ParseIP(raw); host != nil {
			result[host.String()] = struct{}{}
		}
	}
	return result
}

func (p Proxy) clientPolicy(clientIP string) string {
	if p.ClientPolicies != nil {
		if policy := p.ClientPolicies[clientIP]; policy != "" {
			return policy
		}
	}
	return "default"
}

func sourceIP(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return ""
	}
	return host
}

func (p Proxy) clientIP(msg []byte, src string) string {
	if ip := net.ParseIP(src); ip != nil && !ip.IsLoopback() {
		return src
	}
	if ecs := clientSubnetIPv4(msg); ecs != "" {
		return ecs
	}
	return src
}

func clientSubnetIPv4(msg []byte) string {
	if len(msg) < 12 {
		return ""
	}
	qd := int(binary.BigEndian.Uint16(msg[4:6]))
	ar := int(binary.BigEndian.Uint16(msg[10:12]))
	off := 12
	var ok bool
	for i := 0; i < qd; i++ {
		_, off, ok = decodeName(msg, off, 0)
		if !ok || off+4 > len(msg) {
			return ""
		}
		off += 4
	}
	for i := 0; i < ar; i++ {
		_, off, ok = decodeName(msg, off, 0)
		if !ok || off+10 > len(msg) {
			return ""
		}
		rrType := binary.BigEndian.Uint16(msg[off : off+2])
		rdLen := int(binary.BigEndian.Uint16(msg[off+8 : off+10]))
		off += 10
		if off+rdLen > len(msg) {
			return ""
		}
		if rrType == 41 {
			if ip := ecsIPv4FromOptions(msg[off : off+rdLen]); ip != "" {
				return ip
			}
		}
		off += rdLen
	}
	return ""
}

func stripClientSubnetOption(msg []byte) []byte {
	if len(msg) < 12 {
		return msg
	}
	qd := int(binary.BigEndian.Uint16(msg[4:6]))
	an := int(binary.BigEndian.Uint16(msg[6:8]))
	ns := int(binary.BigEndian.Uint16(msg[8:10]))
	ar := int(binary.BigEndian.Uint16(msg[10:12]))

	out := make([]byte, 0, len(msg))
	out = append(out, msg[:12]...)
	off := 12
	changed := false

	for i := 0; i < qd; i++ {
		start := off
		var ok bool
		_, off, ok = decodeName(msg, off, 0)
		if !ok || off+4 > len(msg) {
			return msg
		}
		off += 4
		out = append(out, msg[start:off]...)
	}

	for i := 0; i < an+ns; i++ {
		start := off
		next, ok := resourceRecordEnd(msg, off)
		if !ok {
			return msg
		}
		off = next
		out = append(out, msg[start:off]...)
	}

	for i := 0; i < ar; i++ {
		start := off
		var ok bool
		_, off, ok = decodeName(msg, off, 0)
		if !ok || off+10 > len(msg) {
			return msg
		}
		rrType := binary.BigEndian.Uint16(msg[off : off+2])
		rdLen := int(binary.BigEndian.Uint16(msg[off+8 : off+10]))
		rdataStart := off + 10
		rdataEnd := rdataStart + rdLen
		if rdataEnd > len(msg) {
			return msg
		}
		if rrType != rrTypeOPT {
			out = append(out, msg[start:rdataEnd]...)
			off = rdataEnd
			continue
		}

		options, optionChanged, ok := stripEDNSClientSubnetOptions(msg[rdataStart:rdataEnd])
		if !ok {
			return msg
		}
		if !optionChanged {
			out = append(out, msg[start:rdataEnd]...)
			off = rdataEnd
			continue
		}
		changed = true
		out = append(out, msg[start:off+8]...)
		var lenBuf [2]byte
		binary.BigEndian.PutUint16(lenBuf[:], uint16(len(options)))
		out = append(out, lenBuf[:]...)
		out = append(out, options...)
		off = rdataEnd
	}

	if off != len(msg) {
		return msg
	}
	if !changed {
		return msg
	}
	return out
}

func resourceRecordEnd(msg []byte, off int) (int, bool) {
	var ok bool
	_, off, ok = decodeName(msg, off, 0)
	if !ok || off+10 > len(msg) {
		return 0, false
	}
	rdLen := int(binary.BigEndian.Uint16(msg[off+8 : off+10]))
	off += 10
	if off+rdLen > len(msg) {
		return 0, false
	}
	return off + rdLen, true
}

func stripEDNSClientSubnetOptions(options []byte) ([]byte, bool, bool) {
	out := make([]byte, 0, len(options))
	changed := false
	for off := 0; off < len(options); {
		if off+4 > len(options) {
			return nil, false, false
		}
		code := binary.BigEndian.Uint16(options[off : off+2])
		length := int(binary.BigEndian.Uint16(options[off+2 : off+4]))
		next := off + 4 + length
		if next > len(options) {
			return nil, false, false
		}
		if code == ednsClientSubnetOption {
			changed = true
		} else {
			out = append(out, options[off:next]...)
		}
		off = next
	}
	return out, changed, true
}

func ecsIPv4FromOptions(opts []byte) string {
	for off := 0; off+4 <= len(opts); {
		code := binary.BigEndian.Uint16(opts[off : off+2])
		length := int(binary.BigEndian.Uint16(opts[off+2 : off+4]))
		off += 4
		if off+length > len(opts) {
			return ""
		}
		if code == 8 && length >= 8 {
			family := binary.BigEndian.Uint16(opts[off : off+2])
			prefix := opts[off+2]
			addr := opts[off+4 : off+length]
			if family == 1 && prefix > 0 && len(addr) > 0 {
				var full [4]byte
				copy(full[:], addr)
				return net.IPv4(full[0], full[1], full[2], full[3]).String()
			}
		}
		off += length
	}
	return ""
}

func QueryName(msg []byte) (string, bool) {
	query, ok := ParseQuery(msg)
	if !ok {
		return "", false
	}
	return query.Name, true
}

func ParseQuery(msg []byte) (Query, bool) {
	if len(msg) < 12 {
		return Query{}, false
	}
	if binary.BigEndian.Uint16(msg[4:6]) != 1 {
		return Query{}, false
	}
	name, off, ok := decodeName(msg, 12, 0)
	if !ok || off+4 > len(msg) {
		return Query{}, false
	}
	return Query{
		Name:        name,
		Type:        binary.BigEndian.Uint16(msg[off : off+2]),
		QuestionEnd: off + 4,
	}, true
}

func decodeName(msg []byte, off int, depth int) (string, int, bool) {
	if depth > 8 {
		return "", 0, false
	}
	var labels []string
	for {
		if off >= len(msg) {
			return "", 0, false
		}
		l := int(msg[off])
		off++
		switch {
		case l == 0:
			return strings.Join(labels, "."), off, true
		case l&0xc0 == 0xc0:
			if off >= len(msg) {
				return "", 0, false
			}
			ptr := ((l & 0x3f) << 8) | int(msg[off])
			off++
			suffix, _, ok := decodeName(msg, ptr, depth+1)
			if !ok {
				return "", 0, false
			}
			if suffix != "" {
				labels = append(labels, suffix)
			}
			return strings.Join(labels, "."), off, true
		case l&0xc0 != 0:
			return "", 0, false
		default:
			if off+l > len(msg) {
				return "", 0, false
			}
			labels = append(labels, string(msg[off:off+l]))
			off += l
		}
	}
}

func nodataResponse(query []byte, questionEnd int) []byte {
	resp := make([]byte, questionEnd)
	copy(resp, query[:questionEnd])
	resp[2] = 0x81
	resp[3] = 0x80
	if len(query) >= 4 && query[2]&0x01 == 0x01 {
		resp[2] |= 0x01
	}
	binary.BigEndian.PutUint16(resp[6:8], 0)
	binary.BigEndian.PutUint16(resp[8:10], 0)
	binary.BigEndian.PutUint16(resp[10:12], 0)
	return resp
}

func nxdomainResponse(query []byte, questionEnd int) []byte {
	resp := nodataResponse(query, questionEnd)
	resp[3] = (resp[3] & 0xf0) | 0x03
	return resp
}

func servfailResponse(query []byte) []byte {
	parsed, ok := ParseQuery(query)
	if !ok {
		return nil
	}
	resp := nodataResponse(query, parsed.QuestionEnd)
	resp[3] = (resp[3] & 0xf0) | 0x02
	return resp
}

func readTCPDNS(r io.Reader) ([]byte, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := int(binary.BigEndian.Uint16(hdr[:]))
	if n == 0 {
		return nil, fmt.Errorf("empty DNS TCP message")
	}
	msg := make([]byte, n)
	_, err := io.ReadFull(r, msg)
	return msg, err
}

func writeTCPDNS(w io.Writer, msg []byte) error {
	if len(msg) > 65535 {
		return fmt.Errorf("DNS TCP message too large")
	}
	var hdr [2]byte
	binary.BigEndian.PutUint16(hdr[:], uint16(len(msg)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(msg)
	return err
}
