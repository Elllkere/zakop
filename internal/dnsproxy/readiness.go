package dnsproxy

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

const readinessAttemptTimeout = time.Second

// WaitReady waits until the zakopd DNS listener can complete a real-DNS query.
// This proves that both zakopd and the selected sing-box real-DNS path are ready.
func WaitReady(ctx context.Context, address string) error {
	var lastErr error
	for {
		if err := probeDNS(ctx, address); err == nil {
			return nil
		} else {
			lastErr = err
		}

		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("DNS %s is not ready: %w (last probe: %v)", address, ctx.Err(), lastErr)
		case <-timer.C:
		}
	}
}

func probeDNS(ctx context.Context, address string) error {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "udp", address)
	if err != nil {
		return err
	}
	defer conn.Close()

	deadline := time.Now().Add(readinessAttemptTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}

	id := uint16(time.Now().UnixNano())
	query := readinessQuery(id)
	if _, err := conn.Write(query); err != nil {
		return err
	}
	response := make([]byte, 4096)
	n, err := conn.Read(response)
	if err != nil {
		return err
	}
	response = response[:n]
	if len(response) < 12 {
		return fmt.Errorf("short DNS response")
	}
	if binary.BigEndian.Uint16(response[0:2]) != id || response[2]&0x80 == 0 {
		return fmt.Errorf("invalid DNS response")
	}
	switch rcode := response[3] & 0x0f; rcode {
	case 0, 3:
		return nil
	default:
		return fmt.Errorf("DNS response code %d", rcode)
	}
}

func readinessQuery(id uint16) []byte {
	msg := []byte{
		0, 0, // ID
		0x01, 0x00, // recursion desired
		0x00, 0x01, // one question
		0, 0, 0, 0, 0, 0,
	}
	binary.BigEndian.PutUint16(msg[0:2], id)
	for _, label := range []string{"example", "com"} {
		msg = append(msg, byte(len(label)))
		msg = append(msg, label...)
	}
	return append(msg, 0, 0, 1, 0, 1)
}
