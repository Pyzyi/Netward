package mqtt

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Pyzyi/netzwart/internal/config"
	"github.com/Pyzyi/netzwart/internal/hub"
	"github.com/Pyzyi/netzwart/internal/store"
)

// fakeBroker implements just enough MQTT 3.1.1 to exercise the client.
type fakeBroker struct {
	ln       net.Listener
	mu       sync.Mutex
	retained map[string]string
	connect  []byte
	conns    []net.Conn
}

func newBroker(t *testing.T) *fakeBroker {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	b := &fakeBroker{ln: ln, retained: map[string]string{}}
	t.Cleanup(func() {
		ln.Close()
		b.mu.Lock()
		for _, c := range b.conns {
			c.Close()
		}
		b.mu.Unlock()
	})
	go b.serve()
	return b
}

func (b *fakeBroker) serve() {
	for {
		c, err := b.ln.Accept()
		if err != nil {
			return
		}
		b.mu.Lock()
		b.conns = append(b.conns, c)
		b.mu.Unlock()
		go b.handle(c)
	}
}

func (b *fakeBroker) handle(c net.Conn) {
	r := bufio.NewReader(c)
	for {
		h, body, err := readPacket(r)
		if err != nil {
			return
		}
		switch h >> 4 {
		case pConnect:
			b.mu.Lock()
			b.connect = body
			b.mu.Unlock()
			_, _ = c.Write([]byte{pConnack << 4, 2, 0, 0})
		case pPublish:
			tl := int(binary.BigEndian.Uint16(body[0:2]))
			topic := string(body[2 : 2+tl])
			id := body[2+tl : 4+tl]
			payload := string(body[4+tl:])
			if h&0x01 != 0 {
				b.mu.Lock()
				b.retained[topic] = payload
				b.mu.Unlock()
			}
			_, _ = c.Write(packet(pPuback<<4, append([]byte(nil), id...)))
		case pSubscribe:
			_, _ = c.Write(packet(pSuback<<4, []byte{body[0], body[1], 1}))
			// Home Assistant announces itself right after a subscription.
			msg := appendString(nil, "homeassistant/status")
			msg = append(msg, "online"...)
			_, _ = c.Write(packet(pPublish<<4, msg))
		case pPingreq:
			_, _ = c.Write([]byte{pPingresp << 4, 0})
		case pDisconnect:
			c.Close()
			return
		}
	}
}

func (b *fakeBroker) get(topic string) (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	v, ok := b.retained[topic]
	return v, ok
}

func TestClientRoundTrip(t *testing.T) {
	b := newBroker(t)
	got := make(chan string, 1)
	cl, err := Connect(context.Background(), Options{Addr: b.ln.Addr().String(), ClientID: "test", Username: "u", Password: "p",
		WillTopic: "nw/status", WillMsg: "offline", KeepAlive: 2 * time.Second,
		OnMessage: func(topic string, payload []byte) { got <- topic + "=" + string(payload) }})
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	b.mu.Lock()
	connect := b.connect
	b.mu.Unlock()
	if !strings.Contains(string(connect), "nw/status") || !strings.Contains(string(connect), "offline") {
		t.Fatal("last will missing in CONNECT")
	}
	if err := cl.Publish(context.Background(), "nw/x", []byte("1"), true); err != nil {
		t.Fatal(err)
	}
	if v, _ := b.get("nw/x"); v != "1" {
		t.Fatal("retained publish not stored")
	}
	if err := cl.Subscribe(context.Background(), "homeassistant/status"); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-got:
		if m != "homeassistant/status=online" {
			t.Fatalf("unexpected message %q", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no message delivered")
	}
	// Keep-alive must keep the connection open across several intervals.
	time.Sleep(2500 * time.Millisecond)
	select {
	case <-cl.Done():
		t.Fatalf("connection dropped: %v", cl.Err())
	default:
	}
}

func TestConnRefused(t *testing.T) {
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		r := bufio.NewReader(c)
		_, _, _ = readPacket(r)
		_, _ = c.Write([]byte{pConnack << 4, 2, 0, 5})
	}()
	_, err := Connect(context.Background(), Options{Addr: ln.Addr().String(), ClientID: "x"})
	if _, ok := err.(ErrConnRefused); !ok {
		t.Fatalf("want ErrConnRefused, got %v", err)
	}
}

type fakeSource struct{ devs []store.Device }

func (f fakeSource) Devices() []store.Device { return f.devs }

func TestBridgePublishesWatchedDevices(t *testing.T) {
	b := newBroker(t)
	host, port, _ := net.SplitHostPort(b.ln.Addr().String())
	src := fakeSource{devs: []store.Device{
		{ID: 1, Ident: "3c:22:fb:00:00:01", MAC: "3c:22:fb:00:00:01", Hostname: "Annas-iPhone", Watch: true, Online: true, Vendor: "Apple"},
		{ID: 2, Ident: "3c:22:fb:00:00:02", MAC: "3c:22:fb:00:00:02", Watch: false},
	}}
	// Loopback brokers are only reachable with a permissive dialer in tests.
	br := NewBridge(src, hub.New(), nil, nil, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	cfg := config.MQTT{Enabled: true, Host: host, DiscoveryPrefix: "homeassistant", BaseTopic: "netzwart"}
	cfg.Port = mustAtoi(port)
	br.Configure(cfg, "de")
	br.dial = (&net.Dialer{}).DialContext
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go br.Run(ctx)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && br.Status().State != "connected" {
		time.Sleep(20 * time.Millisecond)
	}
	if br.Status().State != "connected" {
		t.Fatalf("bridge not connected: %+v", br.Status())
	}
	id := ObjectID("3c:22:fb:00:00:01")
	if id != "netzwart_3c22fb000001" {
		t.Fatalf("unexpected object id %q", id)
	}
	cfgMsg, ok := b.get("homeassistant/device_tracker/" + id + "/config")
	if !ok || !strings.Contains(cfgMsg, `"payload_home":"home"`) || !strings.Contains(cfgMsg, "Annas-iPhone") {
		t.Fatalf("discovery config wrong: %q", cfgMsg)
	}
	if st, _ := b.get("netzwart/device/" + id + "/state"); st != "home" {
		t.Fatalf("state = %q", st)
	}
	if _, ok := b.get("homeassistant/device_tracker/" + ObjectID("3c:22:fb:00:00:02") + "/config"); ok {
		t.Fatal("unwatched device published")
	}
	if v, _ := b.get("netzwart/status"); v != "online" {
		t.Fatal("availability not online")
	}
	if v, _ := b.get("netzwart/stats/total"); v != "2" {
		t.Fatalf("stats total = %q", v)
	}
}

func mustAtoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}
