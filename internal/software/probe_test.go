package software

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/netip"
	"testing"
)

// listen starts a TCP server on 127.0.0.1 and points port at it.
func listen(t *testing.T, port *uint16, serve func(net.Conn)) {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	old := *port
	*port = uint16(l.Addr().(*net.TCPAddr).Port)
	t.Cleanup(func() { *port = old })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				serve(c)
			}()
		}
	}()
}

var loopback = netip.MustParseAddr("127.0.0.1")

func TestProbeSSH(t *testing.T) {
	got := make(chan []byte, 1)
	listen(t, &sshPort, func(c net.Conn) {
		_, _ = c.Write([]byte("Welcome\r\nSSH-2.0-OpenSSH_9.2p1 Debian-2+deb12u7\r\n"))
		buf := make([]byte, 16)
		n, _ := c.Read(buf) // the probe must not send anything; returns at close
		got <- buf[:n]
	})
	o := one(t, ProbeSSH(context.Background(), loopback))
	if o.Product != Debian || o.Version != "12" {
		t.Fatalf("got %+v", o)
	}
	if sent := <-got; len(sent) != 0 {
		t.Fatalf("probe sent %q", sent)
	}
}

// challenge builds an SMB2 SESSION_SETUP response carrying an NTLM CHALLENGE.
func challenge(major, minor uint8, build uint16) []byte {
	ch := make([]byte, 56)
	copy(ch, "NTLMSSP\x00")
	binary.LittleEndian.PutUint32(ch[8:], 2)
	binary.LittleEndian.PutUint32(ch[20:], 0x02000000|0x00800000)
	ch[48], ch[49] = major, minor
	binary.LittleEndian.PutUint16(ch[50:], build)
	ch[55] = 15
	sec := append([]byte{0xa1, 0x81, 0x00, 0x30}, ch...) // SPNEGO-ish prefix; the parser searches the signature
	h := smb2Header(1, 1)
	binary.LittleEndian.PutUint32(h[8:], statusMoreProcessed)
	body := make([]byte, 8)
	binary.LittleEndian.PutUint16(body[0:], 9)
	binary.LittleEndian.PutUint16(body[4:], smb2HeaderLen+8)
	binary.LittleEndian.PutUint16(body[6:], uint16(len(sec)))
	return frame(append(append(h, body...), sec...))
}

func fakeSMB(resp []byte, sawNTLM chan<- bool) func(net.Conn) {
	return func(c net.Conn) {
		if _, err := readSMB2Request(c); err != nil {
			return
		}
		neg := smb2Header(0, 0)
		_, _ = c.Write(frame(append(neg, make([]byte, 65)...)))
		req, err := readSMB2Request(c)
		if err != nil {
			return
		}
		if sawNTLM != nil {
			sawNTLM <- bytes.Contains(req, []byte("NTLMSSP\x00\x01\x00\x00\x00"))
		}
		_, _ = c.Write(resp)
		_, _ = io.Copy(io.Discard, c)
	}
}

func readSMB2Request(r io.Reader) ([]byte, error) {
	var nb [4]byte
	if _, err := io.ReadFull(r, nb[:]); err != nil {
		return nil, err
	}
	msg := make([]byte, binary.BigEndian.Uint32(nb[:]))
	_, err := io.ReadFull(r, msg)
	return msg, err
}

func TestProbeSMB(t *testing.T) {
	sawNTLM := make(chan bool, 1)
	listen(t, &smbPort, fakeSMB(challenge(10, 0, 19045), sawNTLM))
	o := one(t, ProbeSMB(context.Background(), loopback))
	if o.Product != Windows || o.Version != "10.0.19045" {
		t.Fatalf("got %+v", o)
	}
	if !<-sawNTLM {
		t.Fatal("session setup did not carry an NTLMSSP NEGOTIATE")
	}
	a := Assess(Input{Now: now, Observations: []Observation{o}})
	if a.Name != "Windows 10 22H2" || a.Status != StatusOutdated {
		t.Fatalf("assessment %+v", a)
	}
}

func TestProbeSMBSambaAndGarbage(t *testing.T) {
	listen(t, &smbPort, fakeSMB(challenge(6, 1, 0), nil))
	if obs := ProbeSMB(context.Background(), loopback); len(obs) != 0 {
		t.Fatalf("Samba placeholder taken as Windows: %+v", obs)
	}
	// Truncated challenge and lying length fields must not panic.
	bad := challenge(10, 0, 26100)
	bad = bad[:len(bad)-20]
	binary.BigEndian.PutUint32(bad, uint32(len(bad)-4))
	if _, _, _, ok := ntlmChallengeVersion(bad[4:]); ok {
		t.Fatal("truncated challenge accepted")
	}
}

func FuzzNTLMChallenge(f *testing.F) {
	f.Add(challenge(10, 0, 26100)[4:])
	f.Fuzz(func(t *testing.T, msg []byte) {
		_, _, _, _ = ntlmChallengeVersion(msg)
	})
}
