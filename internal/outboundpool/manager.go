package outboundpool

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elllkere/neto/internal/config"
)

type API interface {
	Delay(context.Context, string, string, time.Duration) error
	Selected(context.Context, string) (string, error)
	Select(context.Context, string, string) error
}

type Manager struct {
	Pools  []config.OutboundPool
	API    API
	Logf   func(string, ...any)
	mu     sync.Mutex
	active map[string]string
}

func New(cfg config.Config, logf func(string, ...any)) *Manager {
	return &Manager{
		Pools:  append([]config.OutboundPool(nil), cfg.OutboundPools...),
		API:    &clashAPI{address: cfg.Main.PoolController},
		Logf:   logf,
		active: map[string]string{},
	}
}

func (m *Manager) Run(ctx context.Context) error {
	if len(m.Pools) == 0 {
		<-ctx.Done()
		return nil
	}
	var wg sync.WaitGroup
	for _, pool := range m.Pools {
		pool := pool
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.runPool(ctx, pool)
		}()
	}
	<-ctx.Done()
	wg.Wait()
	return nil
}

func (m *Manager) runPool(ctx context.Context, pool config.OutboundPool) {
	retry := time.Second
	interval := time.Duration(pool.CheckInterval) * time.Second
	failed := false
	for {
		selected, err := m.Check(ctx, pool)
		if ctx.Err() != nil {
			return
		}
		delay := interval
		if err != nil {
			if !failed {
				m.logf("outbound pool %s: %v", pool.Tag, err)
			}
			failed = true
			delay = retry
			retry *= 2
			if retry > 10*time.Second {
				retry = 10 * time.Second
			}
		} else {
			if failed {
				m.logf("outbound pool %s health checks recovered", pool.Tag)
			}
			failed = false
			retry = time.Second
			m.noteSelection(pool.Tag, selected)
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}

func (m *Manager) Check(ctx context.Context, pool config.OutboundPool) (string, error) {
	var failures []string
	for _, outbound := range pool.Outbounds {
		testCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		err := m.API.Delay(testCtx, outbound, pool.CheckURL, 10*time.Second)
		cancel()
		if err != nil {
			failures = append(failures, outbound+": "+err.Error())
			continue
		}
		current, err := m.API.Selected(ctx, pool.Tag)
		if err != nil {
			return "", fmt.Errorf("read selection: %w", err)
		}
		if current != outbound {
			if err := m.API.Select(ctx, pool.Tag, outbound); err != nil {
				return "", fmt.Errorf("select %s: %w", outbound, err)
			}
		}
		return outbound, nil
	}
	return "", fmt.Errorf("no healthy outbound: %s", strings.Join(failures, "; "))
}

func (m *Manager) noteSelection(pool string, selected string) {
	m.mu.Lock()
	previous := m.active[pool]
	m.active[pool] = selected
	m.mu.Unlock()
	if previous != selected {
		m.logf("outbound pool %s selected %s", pool, selected)
	}
}

func (m *Manager) logf(format string, args ...any) {
	if m.Logf != nil {
		m.Logf(format, args...)
	}
}

type clashAPI struct {
	address string
}

func (c *clashAPI) Delay(ctx context.Context, outbound string, target string, timeout time.Duration) error {
	path := fmt.Sprintf("/proxies/%s/delay?url=%s&timeout=%d", url.PathEscape(outbound), url.QueryEscape(target), timeout.Milliseconds())
	body, err := c.request(ctx, "GET", path, nil)
	if err != nil {
		return err
	}
	var response struct {
		Delay int64 `json:"delay"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("invalid delay response: %w", err)
	}
	if response.Delay < 1 {
		return fmt.Errorf("invalid delay %d", response.Delay)
	}
	return nil
}

func (c *clashAPI) Selected(ctx context.Context, pool string) (string, error) {
	body, err := c.request(ctx, "GET", "/proxies/"+url.PathEscape(pool), nil)
	if err != nil {
		return "", err
	}
	var response struct {
		Now string `json:"now"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("invalid selector response: %w", err)
	}
	return strings.TrimSpace(response.Now), nil
}

func (c *clashAPI) Select(ctx context.Context, pool string, outbound string) error {
	body, _ := json.Marshal(map[string]string{"name": outbound})
	_, err := c.request(ctx, "PUT", "/proxies/"+url.PathEscape(pool), body)
	return err
}

func (c *clashAPI) request(ctx context.Context, method string, path string, body []byte) ([]byte, error) {
	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", c.address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	deadline := time.Now().Add(12 * time.Second)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)

	var request bytes.Buffer
	fmt.Fprintf(&request, "%s %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n", method, path, c.address)
	if len(body) > 0 {
		fmt.Fprintf(&request, "Content-Type: application/json\r\nContent-Length: %d\r\n", len(body))
	}
	request.WriteString("\r\n")
	request.Write(body)
	if _, err := conn.Write(request.Bytes()); err != nil {
		return nil, err
	}

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	parts := strings.Fields(statusLine)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid HTTP status %q", strings.TrimSpace(statusLine))
	}
	status, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid HTTP status %q", strings.TrimSpace(statusLine))
	}
	headers := map[string]string{}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if line == "\r\n" || line == "\n" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if ok {
			headers[strings.ToLower(strings.TrimSpace(name))] = strings.TrimSpace(value)
		}
	}
	var response []byte
	if strings.Contains(strings.ToLower(headers["transfer-encoding"]), "chunked") {
		response, err = readChunked(reader, 1<<20)
	} else if lengthText := headers["content-length"]; lengthText != "" {
		length, parseErr := strconv.ParseInt(lengthText, 10, 64)
		if parseErr != nil || length < 0 || length > 1<<20 {
			return nil, fmt.Errorf("invalid HTTP content length %q", lengthText)
		}
		response = make([]byte, length)
		_, err = io.ReadFull(reader, response)
	} else {
		response, err = io.ReadAll(io.LimitReader(reader, 1<<20))
	}
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		message := strings.TrimSpace(string(response))
		if message == "" {
			message = "HTTP " + strconv.Itoa(status)
		}
		return nil, fmt.Errorf("%s", message)
	}
	return response, nil
}

func readChunked(reader *bufio.Reader, limit int64) ([]byte, error) {
	var response bytes.Buffer
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		sizeText := strings.TrimSpace(strings.SplitN(line, ";", 2)[0])
		size, err := strconv.ParseInt(sizeText, 16, 64)
		if err != nil || size < 0 {
			return nil, fmt.Errorf("invalid HTTP chunk size %q", sizeText)
		}
		if size == 0 {
			for {
				trailer, err := reader.ReadString('\n')
				if err != nil {
					return nil, err
				}
				if trailer == "\r\n" || trailer == "\n" {
					return response.Bytes(), nil
				}
			}
		}
		if int64(response.Len())+size > limit {
			return nil, fmt.Errorf("HTTP response exceeds %d bytes", limit)
		}
		if _, err := io.CopyN(&response, reader, size); err != nil {
			return nil, err
		}
		terminator := make([]byte, 2)
		if _, err := io.ReadFull(reader, terminator); err != nil {
			return nil, err
		}
		if !bytes.Equal(terminator, []byte("\r\n")) {
			return nil, fmt.Errorf("invalid HTTP chunk terminator")
		}
	}
}
