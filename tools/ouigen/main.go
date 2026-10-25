// Command ouigen converts the IEEE OUI registry (as published by the
// "oui-data" npm package, which mirrors MA-L, MA-M and MA-S) into the
// compact, gzip-compressed table embedded by internal/oui.
//
//	go run ./tools/ouigen -in index.json -out internal/oui/oui.tsv.gz
package main

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// brands maps a lower-case substring of the registered organisation name to
// the name people actually know the vendor by. Checked in order.
var brands = [][2]string{
	{"hewlett packard enterprise", "HPE"},
	{"hewlett packard", "HP"},
	{"hewlett-packard", "HP"},
	{"apple", "Apple"},
	{"huawei", "Huawei"},
	{"honor device", "Honor"},
	{"cisco", "Cisco"},
	{"samsung", "Samsung"},
	{"intel corporate", "Intel"},
	{"zte corporation", "ZTE"},
	{"texas instruments", "Texas Instruments"},
	{"espressif", "Espressif"},
	{"xiaomi", "Xiaomi"},
	{"amazon technologies", "Amazon"},
	{"oppo", "OPPO"},
	{"oneplus", "OnePlus"},
	{"realme", "realme"},
	{"vivo mobile", "vivo"},
	{"dell", "Dell"},
	{"tp-link", "TP-Link"},
	{"google", "Google"},
	{"nest labs", "Google Nest"},
	{"microsoft", "Microsoft"},
	{"asustek", "ASUS"},
	{"sony interactive", "Sony PlayStation"},
	{"sony", "Sony"},
	{"hikvision", "Hikvision"},
	{"netgear", "Netgear"},
	{"lg electronics", "LG"},
	{"lg innotek", "LG Innotek"},
	{"nintendo", "Nintendo"},
	{"d-link", "D-Link"},
	{"avm audiovisuelles", "AVM"},
	{"avm gmbh", "AVM"},
	{"raspberry pi", "Raspberry Pi"},
	{"sonos", "Sonos"},
	{"hon hai", "Foxconn"},
	{"murata", "Murata"},
	{"azurewave", "AzureWave"},
	{"liteon", "Lite-On"},
	{"lite-on", "Lite-On"},
	{"motorola mobility", "Motorola"},
	{"synology", "Synology"},
	{"qnap", "QNAP"},
	{"ubiquiti", "Ubiquiti"},
	{"philips lighting", "Philips Hue"},
	{"signify", "Philips Hue"},
	{"tuya", "Tuya"},
	{"allterco", "Shelly"},
	{"shelly", "Shelly"},
	{"itead", "Sonoff"},
	{"bose", "Bose"},
	{"roku", "Roku"},
	{"nvidia", "NVIDIA"},
	{"lenovo", "Lenovo"},
	{"brother industries", "Brother"},
	{"seiko epson", "Epson"},
	{"canon", "Canon"},
	{"kyocera", "Kyocera"},
	{"xerox", "Xerox"},
	{"lexmark", "Lexmark"},
	{"ring llc", "Ring"},
	{"ecobee", "ecobee"},
	{"wyze", "Wyze"},
	{"reolink", "Reolink"},
	{"dahua", "Dahua"},
	{"axis communications", "Axis"},
	{"tado", "tado°"},
	{"netatmo", "Netatmo"},
	{"sagemcom", "Sagemcom"},
	{"arcadyan", "Arcadyan"},
	{"vodafone", "Vodafone"},
	{"deutsche telekom", "Telekom"},
	{"zyxel", "Zyxel"},
	{"eero", "eero"},
	{"vantiva", "Vantiva"},
	{"technicolor", "Technicolor"},
	{"universal global scientific", "USI"},
	{"pegatron", "Pegatron"},
	{"quanta", "Quanta"},
	{"wistron", "Wistron"},
	{"compal", "Compal"},
	{"giga-byte", "Gigabyte"},
	{"gigabyte", "Gigabyte"},
	{"micro-star", "MSI"},
	{"asrock", "ASRock"},
	{"realtek", "Realtek"},
	{"mediatek", "MediaTek"},
	{"qualcomm", "Qualcomm"},
	{"broadcom", "Broadcom"},
	{"valve", "Valve"},
	{"microchip", "Microchip"},
	{"fugui precision", "Foxconn"},
	{"cloud network technology", "Foxconn"},
	{"nokia", "Nokia"},
	{"juniper", "Juniper"},
	{"commscope", "CommScope"},
	{"arris", "ARRIS"},
	{"silicon laboratories", "Silicon Labs"},
	{"ieee registration authority", "IEEE Registration Authority"},
	{"panasonic", "Panasonic"},
	{"sharp corporation", "Sharp"},
	{"hisense", "Hisense"},
	{"tcl", "TCL"},
	{"vizio", "VIZIO"},
	{"tesla", "Tesla"},
	{"withings", "Withings"},
	{"garmin", "Garmin"},
	{"fitbit", "Fitbit"},
	{"logitech", "Logitech"},
	{"western digital", "Western Digital"},
	{"buffalo", "Buffalo"},
	{"zimaspace", "ZimaSpace"},
	{"icewhale", "IceWhale"},
	{"vmware", "VMware"},
	{"proxmox", "Proxmox"},
	{"xensource", "Xen"},
	{"parallels", "Parallels"},
	{"oracle", "Oracle"},
	{"bosch", "Bosch"},
	{"siemens", "Siemens"},
	{"miele", "Miele"},
	{"vorwerk", "Vorwerk"},
	{"devolo", "devolo"},
	{"gigaset", "Gigaset"},
	{"loewe", "Loewe"},
	{"denon", "Denon"},
	{"yamaha", "Yamaha"},
	{"harman", "Harman"},
	{"teufel", "Teufel"},
	{"nuki", "Nuki"},
	{"homematic", "Homematic"},
	{"eq-3", "eQ-3"},
	{"elgato", "Elgato"},
	{"nanoleaf", "Nanoleaf"},
	{"lifx", "LIFX"},
	{"wiz connected", "WiZ"},
	{"ikea", "IKEA"},
	{"ezviz", "EZVIZ"},
	{"imou", "Imou"},
	{"meross", "Meross"},
	{"govee", "Govee"},
	{"anker", "Anker"},
	{"eufy", "eufy"},
	{"roborock", "Roborock"},
	{"ecovacs", "Ecovacs"},
	{"irobot", "iRobot"},
	{"dyson", "Dyson"},
}

var legal = regexp.MustCompile(`(?i)[\s,.;&]*(\b(co\.?,?\s*ltd|co|company|inc|incorporated|corp|corporation|corporate|ltd|limited|llc|l\.l\.c|gmbh|ag|s\.?a\.?s?|b\.?v|n\.?v|oy|ab|a/s|s\.?p\.?a|s\.?r\.?l|pte|plc|kg|kgaa|co\.?\s*kg|pty|sdn\s*bhd|s\.?a\.? de c\.?v)\b\.?)+\s*$`)

func shorten(full string) string {
	lower := strings.ToLower(full)
	for _, b := range brands {
		if strings.Contains(lower, b[0]) {
			return b[1]
		}
	}
	s := full
	for i := 0; i < 4; i++ {
		t := strings.TrimRight(legal.ReplaceAllString(s, ""), " ,.;&")
		if t == s || t == "" {
			break
		}
		s = t
	}
	if isUpper(s) && len(s) > 5 {
		s = titleCase(s)
	}
	if s == "" {
		return full
	}
	return s
}

func isUpper(s string) bool {
	hasLetter := false
	for _, r := range s {
		if unicode.IsLower(r) {
			return false
		}
		if unicode.IsLetter(r) {
			hasLetter = true
		}
	}
	return hasLetter
}

func titleCase(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		// Keep short all-letter tokens such as "TV" or "LG" upper-case.
		if len(w) <= 3 && !strings.ContainsAny(w, "aeiou") {
			words[i] = strings.ToUpper(w)
			continue
		}
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

func clean(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' || !unicode.IsPrint(r) {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

func main() {
	in := flag.String("in", "", "path to oui-data index.json")
	out := flag.String("out", "internal/oui/oui.tsv.gz", "output file")
	flag.Parse()
	if *in == "" {
		log.Fatal("-in is required")
	}
	raw, err := os.ReadFile(*in)
	if err != nil {
		log.Fatal(err)
	}
	var data map[string]string
	if err := json.Unmarshal(raw, &data); err != nil {
		log.Fatal(err)
	}
	prefixes := make([]string, 0, len(data))
	for k := range data {
		k = strings.ToUpper(k)
		if l := len(k); l != 6 && l != 7 && l != 9 {
			log.Fatalf("unexpected prefix length %q", k)
		}
		prefixes = append(prefixes, k)
	}
	sort.Strings(prefixes)

	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	zw, _ := gzip.NewWriterLevel(f, gzip.BestCompression)
	zw.Header.ModTime = zw.Header.ModTime.UTC() // zero value: reproducible output
	for _, p := range prefixes {
		v := data[p]
		if v == "" {
			v = data[strings.ToLower(p)]
		}
		full := clean(strings.SplitN(v, "\n", 2)[0])
		if full == "" {
			continue
		}
		short := shorten(full)
		if short == full {
			fmt.Fprintf(zw, "%s\t%s\n", p, short)
		} else {
			fmt.Fprintf(zw, "%s\t%s\t%s\n", p, short, full)
		}
	}
	if err := zw.Close(); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %d prefixes to %s", len(prefixes), *out)
}
