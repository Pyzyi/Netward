package proto

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"net/textproto"
	"net/url"
	"strings"
)

// SSDPSearch is an M-SEARCH request for all devices and services.
var SSDPSearch = []byte("M-SEARCH * HTTP/1.1\r\n" +
	"HOST: 239.255.255.250:1900\r\n" +
	"MAN: \"ssdp:discover\"\r\n" +
	"MX: 2\r\n" +
	"ST: ssdp:all\r\n" +
	"USER-AGENT: Linux/1 UPnP/1.1 Netward/1\r\n\r\n")

// SSDPResponse is a decoded M-SEARCH response.
type SSDPResponse struct {
	Location string
	Server   string
	ST       string
	USN      string
}

var errSSDP = errors.New("invalid SSDP response")

// DecodeSSDPResponse parses an HTTP-over-UDP search response.
func DecodeSSDPResponse(p []byte) (SSDPResponse, error) {
	if len(p) > 8192 {
		return SSDPResponse{}, errSSDP
	}
	r := textproto.NewReader(bufio.NewReader(bytes.NewReader(p)))
	line, err := r.ReadLine()
	if err != nil || !strings.HasPrefix(line, "HTTP/1.") || !strings.Contains(line, " 200") {
		return SSDPResponse{}, errSSDP
	}
	h, err := r.ReadMIMEHeader()
	if err != nil && len(h) == 0 {
		return SSDPResponse{}, errSSDP
	}
	return SSDPResponse{
		Location: strings.TrimSpace(h.Get("Location")),
		Server:   strings.TrimSpace(h.Get("Server")),
		ST:       strings.TrimSpace(h.Get("St")),
		USN:      strings.TrimSpace(h.Get("Usn")),
	}, nil
}

// SafeLocation validates an SSDP LOCATION URL before Netward fetches it.
// The URL must be plain HTTP and point at the very address that sent the
// response; anything else could be abused to make Netward request
// arbitrary URLs (SSRF).
func SafeLocation(location string, from netip.Addr) (*url.URL, bool) {
	u, err := url.Parse(location)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Opaque != "" {
		return nil, false
	}
	host := u.Hostname()
	ip, err := netip.ParseAddr(host)
	if err != nil || ip.Unmap() != from.Unmap() {
		return nil, false
	}
	return u, true
}

// UPnPDevice is the subset of a UPnP device description Netward uses.
type UPnPDevice struct {
	DeviceType       string       `xml:"deviceType"`
	FriendlyName     string       `xml:"friendlyName"`
	Manufacturer     string       `xml:"manufacturer"`
	ModelName        string       `xml:"modelName"`
	ModelNumber      string       `xml:"modelNumber"`
	ModelDescription string       `xml:"modelDescription"`
	Devices          []UPnPDevice `xml:"deviceList>device"`
}

// MaxDescription bounds the size of a device description document.
const MaxDescription = 64 << 10

// DecodeUPnPDescription parses a device description document. encoding/xml
// never resolves external entities, so XXE is not possible.
func DecodeUPnPDescription(p []byte) (UPnPDevice, error) {
	if len(p) > MaxDescription {
		return UPnPDevice{}, errors.New("description too large")
	}
	var root struct {
		Device UPnPDevice `xml:"device"`
	}
	d := xml.NewDecoder(bytes.NewReader(p))
	d.Strict = false
	d.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	if err := d.Decode(&root); err != nil {
		return UPnPDevice{}, err
	}
	return root.Device, nil
}

// AllTypes returns the device types of the root device and all embedded devices.
func (d UPnPDevice) AllTypes() []string {
	var out []string
	var walk func(UPnPDevice, int)
	walk = func(x UPnPDevice, depth int) {
		if depth > 4 {
			return
		}
		if x.DeviceType != "" {
			out = append(out, x.DeviceType)
		}
		for _, c := range x.Devices {
			walk(c, depth+1)
		}
	}
	walk(d, 0)
	return out
}

// NoRedirect is an http.Client CheckRedirect that refuses all redirects.
func NoRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
