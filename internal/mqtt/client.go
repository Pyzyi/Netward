// Package mqtt contains a small MQTT 3.1.1 client (QoS 0/1, retained
// messages, last will, subscribe, TLS, keep-alive) and the Home Assistant
// bridge built on it.
package mqtt

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// Packet types.
const (
	pConnect     = 1
	pConnack     = 2
	pPublish     = 3
	pPuback      = 4
	pSubscribe   = 8
	pSuback      = 9
	pPingreq     = 12
	pPingresp    = 13
	pDisconnect  = 14
	maxPacketLen = 256 << 10
)

// Options configure a connection.
type Options struct {
	Addr       string // host:port
	TLS        bool
	ServerName string
	ClientID   string
	Username   string
	Password   string
	KeepAlive  time.Duration
	WillTopic  string
	WillMsg    string
	Dial       func(ctx context.Context, network, addr string) (net.Conn, error)
	OnMessage  func(topic string, payload []byte)
}

// Client is a connected MQTT session. It is safe for concurrent use.
type Client struct {
	opt  Options
	conn net.Conn
	w    sync.Mutex // serialises writes

	mu      sync.Mutex
	nextID  uint16
	pending map[uint16]chan struct{}
	closed  chan struct{}
	err     error
	pong    chan struct{}
}

// ErrConnRefused is returned when the broker rejects the connection.
type ErrConnRefused struct{ Code byte }

func (e ErrConnRefused) Error() string {
	reasons := map[byte]string{1: "unacceptable protocol version", 2: "client ID rejected", 3: "server unavailable",
		4: "bad user name or password", 5: "not authorised"}
	if r, ok := reasons[e.Code]; ok {
		return "broker refused connection: " + r
	}
	return fmt.Sprintf("broker refused connection (code %d)", e.Code)
}

func appendString(b []byte, s string) []byte {
	b = binary.BigEndian.AppendUint16(b, uint16(len(s)))
	return append(b, s...)
}

func appendLength(b []byte, n int) []byte {
	for {
		d := byte(n % 128)
		n /= 128
		if n > 0 {
			d |= 0x80
		}
		b = append(b, d)
		if n == 0 {
			return b
		}
	}
}

func packet(header byte, body []byte) []byte {
	out := []byte{header}
	out = appendLength(out, len(body))
	return append(out, body...)
}

func readPacket(r *bufio.Reader) (byte, []byte, error) {
	h, err := r.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	n, mult := 0, 1
	for i := 0; ; i++ {
		if i == 4 {
			return 0, nil, errors.New("mqtt: malformed length")
		}
		b, err := r.ReadByte()
		if err != nil {
			return 0, nil, err
		}
		n += int(b&0x7f) * mult
		if b&0x80 == 0 {
			break
		}
		mult *= 128
	}
	if n > maxPacketLen {
		return 0, nil, errors.New("mqtt: packet too large")
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return 0, nil, err
	}
	return h, body, nil
}

// Connect dials the broker and performs the MQTT handshake.
func Connect(ctx context.Context, opt Options) (*Client, error) {
	if opt.KeepAlive == 0 {
		opt.KeepAlive = 60 * time.Second
	}
	if opt.Dial == nil {
		opt.Dial = (&net.Dialer{Timeout: 10 * time.Second}).DialContext
	}
	conn, err := opt.Dial(ctx, "tcp", opt.Addr)
	if err != nil {
		return nil, err
	}
	if opt.TLS {
		sn := opt.ServerName
		if sn == "" {
			sn, _, _ = net.SplitHostPort(opt.Addr)
		}
		tc := tls.Client(conn, &tls.Config{ServerName: sn, MinVersion: tls.VersionTLS12})
		if err := tc.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, err
		}
		conn = tc
	}
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	flags := byte(0x02) // clean session
	body := appendString(nil, "MQTT")
	body = append(body, 4) // protocol level 3.1.1
	var payload []byte
	payload = appendString(payload, opt.ClientID)
	if opt.WillTopic != "" {
		flags |= 0x04 | 0x08 | 0x20 // will, QoS 1, retain
		payload = appendString(payload, opt.WillTopic)
		payload = appendString(payload, opt.WillMsg)
	}
	if opt.Username != "" {
		flags |= 0x80
		payload = appendString(payload, opt.Username)
		if opt.Password != "" {
			flags |= 0x40
			payload = appendString(payload, opt.Password)
		}
	}
	body = append(body, flags)
	body = binary.BigEndian.AppendUint16(body, uint16(opt.KeepAlive/time.Second))
	body = append(body, payload...)
	if _, err := conn.Write(packet(pConnect<<4, body)); err != nil {
		conn.Close()
		return nil, err
	}
	r := bufio.NewReader(conn)
	h, ack, err := readPacket(r)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if h>>4 != pConnack || len(ack) != 2 {
		conn.Close()
		return nil, errors.New("mqtt: unexpected reply to CONNECT")
	}
	if ack[1] != 0 {
		conn.Close()
		return nil, ErrConnRefused{Code: ack[1]}
	}
	_ = conn.SetDeadline(time.Time{})
	c := &Client{opt: opt, conn: conn, pending: map[uint16]chan struct{}{}, closed: make(chan struct{}), pong: make(chan struct{}, 1)}
	go c.readLoop(r)
	go c.keepAlive()
	return c, nil
}

func (c *Client) write(b []byte) error {
	c.w.Lock()
	defer c.w.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := c.conn.Write(b)
	return err
}

func (c *Client) fail(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-c.closed:
		return
	default:
	}
	c.err = err
	close(c.closed)
	c.conn.Close()
}

// Done is closed when the connection ends.
func (c *Client) Done() <-chan struct{} { return c.closed }

// Err returns why the connection ended.
func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *Client) readLoop(r *bufio.Reader) {
	for {
		h, body, err := readPacket(r)
		if err != nil {
			c.fail(err)
			return
		}
		switch h >> 4 {
		case pPuback, pSuback:
			if len(body) >= 2 {
				id := binary.BigEndian.Uint16(body[0:2])
				c.mu.Lock()
				ch := c.pending[id]
				delete(c.pending, id)
				c.mu.Unlock()
				if ch != nil {
					close(ch)
				}
			}
		case pPingresp:
			select {
			case c.pong <- struct{}{}:
			default:
			}
		case pPublish:
			qos := (h >> 1) & 0x03
			if len(body) < 2 {
				continue
			}
			tl := int(binary.BigEndian.Uint16(body[0:2]))
			if 2+tl > len(body) {
				continue
			}
			topic := string(body[2 : 2+tl])
			rest := body[2+tl:]
			if qos > 0 {
				if len(rest) < 2 {
					continue
				}
				id := rest[0:2]
				rest = rest[2:]
				_ = c.write(packet(pPuback<<4, append([]byte(nil), id...)))
			}
			if c.opt.OnMessage != nil {
				c.opt.OnMessage(topic, rest)
			}
		}
	}
}

func (c *Client) keepAlive() {
	t := time.NewTicker(c.opt.KeepAlive / 2)
	defer t.Stop()
	for {
		select {
		case <-c.closed:
			return
		case <-t.C:
		}
		if err := c.write([]byte{pPingreq << 4, 0}); err != nil {
			c.fail(err)
			return
		}
		select {
		case <-c.pong:
		case <-time.After(c.opt.KeepAlive / 2):
			c.fail(errors.New("mqtt: keep-alive timeout"))
			return
		case <-c.closed:
			return
		}
	}
}

func (c *Client) newID() (uint16, chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for {
		c.nextID++
		if c.nextID == 0 {
			c.nextID = 1
		}
		if _, used := c.pending[c.nextID]; !used {
			ch := make(chan struct{})
			c.pending[c.nextID] = ch
			return c.nextID, ch
		}
	}
}

func (c *Client) await(ctx context.Context, id uint16, ch chan struct{}) error {
	select {
	case <-ch:
		return nil
	case <-c.closed:
		return errors.New("mqtt: connection closed")
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return ctx.Err()
	case <-time.After(10 * time.Second):
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return errors.New("mqtt: acknowledgement timeout")
	}
}

// Publish sends a QoS 1 message and waits for PUBACK.
func (c *Client) Publish(ctx context.Context, topic string, payload []byte, retain bool) error {
	id, ch := c.newID()
	h := byte(pPublish<<4) | 0x02 // QoS 1
	if retain {
		h |= 0x01
	}
	body := appendString(nil, topic)
	body = binary.BigEndian.AppendUint16(body, id)
	body = append(body, payload...)
	if err := c.write(packet(h, body)); err != nil {
		c.fail(err)
		return err
	}
	return c.await(ctx, id, ch)
}

// Subscribe subscribes to a topic filter with QoS 1.
func (c *Client) Subscribe(ctx context.Context, filter string) error {
	id, ch := c.newID()
	body := binary.BigEndian.AppendUint16(nil, id)
	body = appendString(body, filter)
	body = append(body, 1)
	if err := c.write(packet(pSubscribe<<4|0x02, body)); err != nil {
		c.fail(err)
		return err
	}
	return c.await(ctx, id, ch)
}

// Close sends DISCONNECT and closes the connection (no last will is sent).
func (c *Client) Close() {
	_ = c.write([]byte{pDisconnect << 4, 0})
	c.fail(errors.New("closed"))
}
