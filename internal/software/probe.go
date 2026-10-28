package software

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"strings"
	"time"
)

// Ports are variables so tests can run fake servers on unprivileged ports.
var (
	sshPort uint16 = 22
	smbPort uint16 = 445
)

const probeTimeout = 3 * time.Second

func dial(ctx context.Context, ip netip.Addr, port uint16) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	var d net.Dialer
	c, err := d.DialContext(ctx, "tcp4", netip.AddrPortFrom(ip, port).String())
	if err != nil {
		return nil, err
	}
	_ = c.SetDeadline(time.Now().Add(probeTimeout))
	return c, nil
}

// ProbeSSH reads the identification line an SSH server sends on connect.
// Nothing is written to the server.
func ProbeSSH(ctx context.Context, ip netip.Addr) []Observation {
	c, err := dial(ctx, ip, sshPort)
	if err != nil {
		return nil
	}
	defer c.Close()
	// RFC 4253 allows other lines before the identification; read at most 1 KiB.
	r := bufio.NewReaderSize(io.LimitReader(c, 1024), 1024)
	for i := 0; i < 8; i++ {
		line, err := r.ReadString('\n')
		if strings.HasPrefix(line, "SSH-") {
			return FromSSHBanner(line)
		}
		if err != nil {
			return nil
		}
	}
	return nil
}

// ProbeSMB starts an SMB2 session setup with an NTLMSSP NEGOTIATE message and
// reads the version field of the server's CHALLENGE. No user name or password
// is sent and the challenge is never answered, so there is no logon attempt.
func ProbeSMB(ctx context.Context, ip netip.Addr) []Observation {
	c, err := dial(ctx, ip, smbPort)
	if err != nil {
		return nil
	}
	defer c.Close()
	if _, err := c.Write(smb2Negotiate()); err != nil {
		return nil
	}
	if _, err := readSMB2(c, 0); err != nil {
		return nil
	}
	if _, err := c.Write(smb2SessionSetup()); err != nil {
		return nil
	}
	msg, err := readSMB2(c, 1)
	if err != nil {
		return nil
	}
	maj, mnr, build, ok := ntlmChallengeVersion(msg)
	if !ok {
		return nil
	}
	return FromNTLMVersion(maj, mnr, build)
}

const (
	smb2HeaderLen       = 64
	maxSMBMessage       = 4096
	statusMoreProcessed = 0xC0000016
)

func smb2Header(command uint16, messageID uint64) []byte {
	h := make([]byte, smb2HeaderLen)
	copy(h, "\xfeSMB")
	binary.LittleEndian.PutUint16(h[4:], smb2HeaderLen)
	binary.LittleEndian.PutUint16(h[12:], command)
	binary.LittleEndian.PutUint16(h[14:], 1) // credits requested
	binary.LittleEndian.PutUint64(h[24:], messageID)
	return h
}

// frame prepends the 4-byte NetBIOS session header.
func frame(msg []byte) []byte {
	out := make([]byte, 4, 4+len(msg))
	binary.BigEndian.PutUint32(out, uint32(len(msg)))
	return append(out, msg...)
}

func smb2Negotiate() []byte {
	dialects := []uint16{0x0202, 0x0210, 0x0300, 0x0302} // no 3.1.1: no negotiate contexts needed
	body := make([]byte, 36, 36+2*len(dialects))
	binary.LittleEndian.PutUint16(body[0:], 36)
	binary.LittleEndian.PutUint16(body[2:], uint16(len(dialects)))
	binary.LittleEndian.PutUint16(body[4:], 1) // signing enabled
	copy(body[12:28], "Netward-probe-1")       // client GUID
	for _, d := range dialects {
		body = binary.LittleEndian.AppendUint16(body, d)
	}
	return frame(append(smb2Header(0, 0), body...))
}

// ntlmNegotiate is an NTLMSSP NEGOTIATE message without domain or workstation.
func ntlmNegotiate() []byte {
	const flags = 0x00000001 | // unicode
		0x00000004 | // request target
		0x00000200 | // NTLM
		0x00008000 | // always sign
		0x00080000 | // extended session security
		0x00800000 | // target info
		0x02000000 | // version
		0x20000000 | // 128-bit
		0x40000000 | // key exchange
		0x80000000 // 56-bit
	m := make([]byte, 40)
	copy(m, "NTLMSSP\x00")
	binary.LittleEndian.PutUint32(m[8:], 1)
	binary.LittleEndian.PutUint32(m[12:], flags)
	return m
}

// spnegoInit wraps an NTLMSSP token in a GSS-API/SPNEGO NegTokenInit.
func spnegoInit(token []byte) []byte {
	tlv := func(tag byte, content []byte) []byte {
		return append([]byte{tag, byte(len(content))}, content...) // all lengths < 128
	}
	ntlmOID := []byte{0x06, 0x0a, 0x2b, 0x06, 0x01, 0x04, 0x01, 0x82, 0x37, 0x02, 0x02, 0x0a}
	mechTypes := tlv(0xa0, tlv(0x30, ntlmOID))
	mechToken := tlv(0xa2, tlv(0x04, token))
	negTokenInit := tlv(0xa0, tlv(0x30, append(mechTypes, mechToken...)))
	spnegoOID := []byte{0x06, 0x06, 0x2b, 0x06, 0x01, 0x05, 0x05, 0x02}
	return tlv(0x60, append(spnegoOID, negTokenInit...))
}

func smb2SessionSetup() []byte {
	sec := spnegoInit(ntlmNegotiate())
	body := make([]byte, 24, 24+len(sec))
	binary.LittleEndian.PutUint16(body[0:], 25)
	body[3] = 1 // signing enabled
	binary.LittleEndian.PutUint16(body[12:], smb2HeaderLen+24)
	binary.LittleEndian.PutUint16(body[14:], uint16(len(sec)))
	body = append(body, sec...)
	return frame(append(smb2Header(1, 1), body...))
}

var errSMB = errors.New("unexpected SMB response")

// readSMB2 reads one framed SMB2 response to messageID and returns it
// (header included). Responses larger than maxSMBMessage are rejected.
func readSMB2(r io.Reader, messageID uint64) ([]byte, error) {
	var nb [4]byte
	if _, err := io.ReadFull(r, nb[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(nb[:])
	if nb[0] != 0 || n < smb2HeaderLen || n > maxSMBMessage {
		return nil, errSMB
	}
	msg := make([]byte, n)
	if _, err := io.ReadFull(r, msg); err != nil {
		return nil, err
	}
	if !bytes.Equal(msg[:4], []byte("\xfeSMB")) || binary.LittleEndian.Uint64(msg[24:]) != messageID {
		return nil, errSMB
	}
	status := binary.LittleEndian.Uint32(msg[8:])
	if status != 0 && status != statusMoreProcessed {
		return nil, errSMB
	}
	return msg, nil
}

// ntlmChallengeVersion finds the NTLMSSP CHALLENGE inside a session setup
// response and returns its version field (product major, minor and build).
func ntlmChallengeVersion(msg []byte) (uint8, uint8, uint16, bool) {
	if len(msg) < smb2HeaderLen+8 || binary.LittleEndian.Uint32(msg[8:]) != statusMoreProcessed {
		return 0, 0, 0, false
	}
	body := msg[smb2HeaderLen:]
	off := int(binary.LittleEndian.Uint16(body[4:]))
	ln := int(binary.LittleEndian.Uint16(body[6:]))
	if off < smb2HeaderLen || off+ln > len(msg) {
		return 0, 0, 0, false
	}
	sec := msg[off : off+ln]
	i := bytes.Index(sec, []byte("NTLMSSP\x00"))
	if i < 0 {
		return 0, 0, 0, false
	}
	ch := sec[i:]
	if len(ch) < 56 || binary.LittleEndian.Uint32(ch[8:]) != 2 {
		return 0, 0, 0, false
	}
	if binary.LittleEndian.Uint32(ch[20:])&0x02000000 == 0 {
		return 0, 0, 0, false // no version field
	}
	return ch[48], ch[49], binary.LittleEndian.Uint16(ch[50:]), true
}
