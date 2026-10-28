// Package classify infers what kind of device something is from the clues
// discovery collected. Every clue votes for a type with a weight; the type
// with the highest total wins. The rules are tables so they are easy to
// extend and to test.
package classify

import (
	"strings"
)

// Types, in the order the UI groups them.
const (
	Router      = "router"
	AccessPoint = "access_point"
	Switch      = "switch"
	Server      = "server"
	NAS         = "nas"
	Computer    = "computer"
	Laptop      = "laptop"
	Phone       = "phone"
	Tablet      = "tablet"
	Watch       = "watch"
	TV          = "tv"
	Streaming   = "streaming"
	Speaker     = "speaker"
	GameConsole = "game_console"
	Printer     = "printer"
	Camera      = "camera"
	SmartHome   = "smart_home"
	Light       = "light"
	Plug        = "plug"
	IoT         = "iot"
	Virtual     = "virtual"
	Unknown     = "unknown"
)

// All lists every valid type.
var All = []string{Router, AccessPoint, Switch, Server, NAS, Computer, Laptop, Phone, Tablet, Watch, TV,
	Streaming, Speaker, GameConsole, Printer, Camera, SmartHome, Light, Plug, IoT, Virtual, Unknown}

// Valid reports whether t is a known type.
func Valid(t string) bool {
	for _, x := range All {
		if x == t {
			return true
		}
	}
	return false
}

// Mobile reports whether devices of type t sleep aggressively on Wi-Fi.
func Mobile(t string) bool { return t == Phone || t == Tablet || t == Watch }

// Input is everything known about a device.
type Input struct {
	Vendor     string
	Hostnames  []string // DHCP, mDNS, NetBIOS, DNS, ...
	Services   []string // mDNS service types ("_airplay._tcp")
	Model      string   // mDNS TXT / UPnP model name
	UPnPTypes  []string // UPnP device types
	UPnPMaker  string
	DHCPVendor string // DHCP option 60
	DHCPParams string // DHCP option 55 as "1,3,6"
	HomeKitCat string // HomeKit "ci" TXT value
	OpenPorts  []int
	IsGateway  bool
	IsSelf     bool
	MACKind    string // universal | random | virtual
}

// Result is the classification outcome.
type Result struct {
	Type       string
	Confidence int // 0-100
	OS         string
}

type rule struct {
	needle string
	typ    string
	weight int
}

// hostnameRules match lower-cased host names (substring).
var hostnameRules = []rule{
	{"iphone", Phone, 60}, {"ipad", Tablet, 60}, {"macbook", Laptop, 60}, {"imac", Computer, 55},
	{"mac-mini", Computer, 55}, {"macmini", Computer, 55}, {"mac-studio", Computer, 55}, {"apple-watch", Watch, 60},
	{"watch", Watch, 25}, {"appletv", Streaming, 55}, {"apple-tv", Streaming, 55}, {"homepod", Speaker, 60},
	{"galaxy", Phone, 40}, {"pixel", Phone, 40}, {"android", Phone, 35}, {"oneplus", Phone, 45}, {"redmi", Phone, 45},
	{"xiaomi", Phone, 20}, {"huawei", Phone, 20}, {"oppo", Phone, 35}, {"realme", Phone, 40}, {"moto", Phone, 25},
	{"desktop-", Computer, 45}, {"laptop-", Laptop, 50}, {"notebook", Laptop, 45}, {"thinkpad", Laptop, 55},
	{"surface", Laptop, 40}, {"pc-", Computer, 30}, {"workstation", Computer, 40}, {"ubuntu", Computer, 25},
	{"ps4", GameConsole, 60}, {"ps5", GameConsole, 60}, {"playstation", GameConsole, 60}, {"xbox", GameConsole, 60},
	{"nintendo", GameConsole, 55}, {"switch", GameConsole, 20}, {"steamdeck", GameConsole, 55}, {"steam-deck", GameConsole, 55},
	{"echo", Speaker, 30}, {"alexa", Speaker, 45}, {"sonos", Speaker, 55}, {"chromecast", Streaming, 60},
	{"firetv", Streaming, 55}, {"fire-tv", Streaming, 55}, {"aft", Streaming, 15}, {"roku", Streaming, 55},
	{"shield", Streaming, 30}, {"tv", TV, 20}, {"bravia", TV, 55}, {"tizen", TV, 50}, {"webos", TV, 50}, {"lgwebos", TV, 55},
	{"printer", Printer, 50}, {"drucker", Printer, 50}, {"brn", Printer, 35}, {"npi", Printer, 40}, {"hp", Printer, 10},
	{"epson", Printer, 45}, {"canon", Printer, 35}, {"laserjet", Printer, 60}, {"officejet", Printer, 60},
	{"cam", Camera, 30}, {"camera", Camera, 50}, {"ipcam", Camera, 55}, {"doorbell", Camera, 45}, {"reolink", Camera, 50},
	{"nas", NAS, 45}, {"diskstation", NAS, 60}, {"synology", NAS, 50}, {"qnap", NAS, 50}, {"truenas", NAS, 55},
	{"unraid", NAS, 55}, {"zimaos", NAS, 50}, {"casaos", NAS, 45}, {"zimacube", NAS, 55}, {"zimaboard", Server, 50},
	{"raspberrypi", Server, 45}, {"pihole", Server, 50}, {"homeassistant", SmartHome, 55}, {"home-assistant", SmartHome, 55},
	{"proxmox", Server, 55}, {"server", Server, 35}, {"srv", Server, 25},
	{"esp_", IoT, 45}, {"esp-", IoT, 40}, {"espressif", IoT, 45}, {"tasmota", Plug, 40}, {"shelly", Plug, 40},
	{"sonoff", Plug, 45}, {"tuya", IoT, 30}, {"hue", Light, 25}, {"bulb", Light, 45}, {"lamp", Light, 35},
	{"wled", Light, 50}, {"nanoleaf", Light, 50}, {"tado", SmartHome, 45}, {"netatmo", SmartHome, 45},
	{"nuki", SmartHome, 45}, {"bridge", SmartHome, 25}, {"gateway", SmartHome, 15}, {"ccu", SmartHome, 40},
	{"fritz", Router, 45}, {"fritzbox", Router, 55}, {"fritz-repeater", AccessPoint, 60}, {"repeater", AccessPoint, 55},
	{"speedport", Router, 55}, {"router", Router, 45}, {"unifi", AccessPoint, 40}, {"u6-", AccessPoint, 50},
	{"uap", AccessPoint, 45}, {"ap-", AccessPoint, 30}, {"mesh", AccessPoint, 30}, {"deco", AccessPoint, 40},
	{"eero", AccessPoint, 45}, {"orbi", AccessPoint, 45}, {"switch-", Switch, 40}, {"usw", Switch, 45},
	{"kindle", Tablet, 45}, {"tolino", Tablet, 45},
}

// vendorRules match lower-cased vendor names (substring).
var vendorRules = []rule{
	{"avm", Router, 30}, {"sagemcom", Router, 30}, {"arcadyan", Router, 25}, {"technicolor", Router, 25},
	{"vantiva", Router, 25}, {"zyxel", Router, 20}, {"draytek", Router, 30}, {"mikrotik", Router, 25},
	{"ubiquiti", AccessPoint, 25}, {"tp-link", AccessPoint, 10}, {"netgear", AccessPoint, 10},
	{"eero", AccessPoint, 35}, {"ruckus", AccessPoint, 35}, {"aruba", AccessPoint, 30}, {"devolo", AccessPoint, 35},
	{"synology", NAS, 45}, {"qnap", NAS, 45}, {"western digital", NAS, 20}, {"buffalo", NAS, 20},
	{"zimaspace", NAS, 40}, {"icewhale", NAS, 40},
	{"raspberry pi", Server, 25},
	{"sonos", Speaker, 45}, {"bose", Speaker, 40}, {"denon", Speaker, 30}, {"yamaha", Speaker, 20},
	{"harman", Speaker, 25}, {"teufel", Speaker, 40},
	{"roku", Streaming, 40}, {"sony playstation", GameConsole, 50}, {"nintendo", GameConsole, 45}, {"valve", GameConsole, 30},
	{"brother", Printer, 40}, {"epson", Printer, 35}, {"canon", Printer, 25}, {"kyocera", Printer, 45},
	{"xerox", Printer, 45}, {"lexmark", Printer, 45}, {"ricoh", Printer, 40},
	{"hikvision", Camera, 45}, {"dahua", Camera, 45}, {"reolink", Camera, 45}, {"axis", Camera, 40},
	{"ezviz", Camera, 40}, {"imou", Camera, 40}, {"ring", Camera, 30}, {"wyze", Camera, 30}, {"eufy", Camera, 25},
	{"philips hue", Light, 35}, {"lifx", Light, 45}, {"nanoleaf", Light, 45}, {"wiz", Light, 35}, {"govee", Light, 30},
	{"shelly", Plug, 35}, {"sonoff", Plug, 35}, {"meross", Plug, 35}, {"tuya", IoT, 30}, {"espressif", IoT, 30},
	{"silicon labs", IoT, 15}, {"tado", SmartHome, 40}, {"netatmo", SmartHome, 40}, {"ecobee", SmartHome, 40},
	{"nuki", SmartHome, 40}, {"homematic", SmartHome, 40}, {"eq-3", SmartHome, 40},
	{"roborock", IoT, 35}, {"ecovacs", IoT, 35}, {"irobot", IoT, 35}, {"dyson", IoT, 30}, {"miele", IoT, 30},
	{"vorwerk", IoT, 30}, {"bosch", IoT, 10}, {"siemens", IoT, 10},
	{"vmware", Virtual, 50}, {"proxmox", Virtual, 40}, {"parallels", Virtual, 50}, {"xen", Virtual, 40},
	{"intel", Computer, 10}, {"realtek", Computer, 5}, {"asus", Computer, 10}, {"gigabyte", Computer, 15},
	{"msi", Computer, 15}, {"asrock", Computer, 15}, {"dell", Computer, 15}, {"lenovo", Computer, 10},
	{"hp", Computer, 5}, {"microsoft", Computer, 10}, {"liteon", Laptop, 10}, {"azurewave", Laptop, 10},
	{"apple", Phone, 5}, {"samsung", Phone, 5}, {"amazon", Speaker, 10}, {"google", Streaming, 5},
	{"lg", TV, 5}, {"hisense", TV, 30}, {"tcl", TV, 25}, {"vizio", TV, 40}, {"panasonic", TV, 10}, {"loewe", TV, 40},
	{"tesla", IoT, 25}, {"garmin", Watch, 20}, {"fitbit", Watch, 30}, {"withings", IoT, 30},
}

// serviceRules match mDNS service types exactly.
var serviceRules = map[string][]rule{
	"_airplay._tcp":          {{"", Streaming, 20}},
	"_raop._tcp":             {{"", Speaker, 15}},
	"_googlecast._tcp":       {{"", Streaming, 40}},
	"_spotify-connect._tcp":  {{"", Speaker, 25}},
	"_sonos._tcp":            {{"", Speaker, 50}},
	"_ipp._tcp":              {{"", Printer, 45}},
	"_ipps._tcp":             {{"", Printer, 45}},
	"_printer._tcp":          {{"", Printer, 45}},
	"_pdl-datastream._tcp":   {{"", Printer, 45}},
	"_scanner._tcp":          {{"", Printer, 30}},
	"_uscan._tcp":            {{"", Printer, 30}},
	"_hap._tcp":              {{"", SmartHome, 20}},
	"_hue._tcp":              {{"", SmartHome, 40}},
	"_esphomelib._tcp":       {{"", IoT, 40}},
	"_shelly._tcp":           {{"", Plug, 40}},
	"_home-assistant._tcp":   {{"", SmartHome, 50}},
	"_smb._tcp":              {{"", Computer, 10}},
	"_afpovertcp._tcp":       {{"", NAS, 15}},
	"_adisk._tcp":            {{"", NAS, 30}},
	"_nfs._tcp":              {{"", NAS, 25}},
	"_ssh._tcp":              {{"", Server, 10}},
	"_workstation._tcp":      {{"", Computer, 15}},
	"_companion-link._tcp":   {{"", Phone, 10}},
	"_apple-mobdev2._tcp":    {{"", Phone, 30}},
	"_amzn-wplay._tcp":       {{"", Streaming, 35}},
	"_androidtvremote2._tcp": {{"", TV, 40}},
	"_nvstream._tcp":         {{"", Computer, 20}},
	"_rfb._tcp":              {{"", Computer, 10}},
	"_daap._tcp":             {{"", Computer, 10}},
	"_matter._tcp":           {{"", SmartHome, 25}},
	"_matterc._udp":          {{"", SmartHome, 25}},
}

// modelRules match lower-cased model strings (prefix).
var modelRules = []rule{
	{"iphone", Phone, 80}, {"ipad", Tablet, 80}, {"watch", Watch, 70}, {"macbook", Laptop, 80},
	{"imac", Computer, 75}, {"macmini", Computer, 75}, {"mac1", Computer, 50}, {"mac14", Computer, 50},
	{"macpro", Computer, 75}, {"appletv", Streaming, 80}, {"audioaccessory", Speaker, 80}, {"airport", AccessPoint, 70},
	{"chromecast", Streaming, 80}, {"google home", Speaker, 75}, {"google nest hub", SmartHome, 60},
	{"nest audio", Speaker, 75}, {"nest mini", Speaker, 75}, {"sonos", Speaker, 70},
	{"bravia", TV, 70}, {"qe", TV, 20}, {"ue", TV, 15}, {"oled", TV, 50},
	{"ds", NAS, 15}, {"rs", NAS, 10},
}

// homeKitCategories maps HomeKit accessory categories (ci) to types.
var homeKitCategories = map[string]string{
	"2": SmartHome, "5": Light, "7": Plug, "8": Plug, "9": SmartHome, "10": SmartHome,
	"17": Camera, "18": Camera, "26": Speaker, "31": TV, "32": TV, "36": Streaming,
}

// portRules give weight to characteristic open ports.
var portRules = map[int][]rule{
	9100:  {{"", Printer, 40}},
	631:   {{"", Printer, 20}},
	515:   {{"", Printer, 25}},
	554:   {{"", Camera, 30}},
	8554:  {{"", Camera, 20}},
	5000:  {{"", NAS, 10}},
	5001:  {{"", NAS, 20}},
	62078: {{"", Phone, 45}},
	8123:  {{"", SmartHome, 50}},
	32400: {{"", Server, 30}},
	3389:  {{"", Computer, 30}},
	445:   {{"", Computer, 10}},
	22:    {{"", Server, 10}},
	8008:  {{"", Streaming, 20}},
	8009:  {{"", Streaming, 25}},
	1400:  {{"", Speaker, 40}},
	7000:  {{"", Streaming, 10}},
	53:    {{"", Server, 10}},
	1883:  {{"", Server, 10}},
	2375:  {{"", Server, 20}},
	9000:  {{"", Server, 10}},
	8006:  {{"", Server, 45}},
}

// upnpRules match UPnP device type URNs (substring).
var upnpRules = []rule{
	{"internetgatewaydevice", Router, 60}, {"wanconnectiondevice", Router, 40}, {"wlanaccesspointdevice", AccessPoint, 40},
	{"mediarenderer", TV, 30}, {"mediaserver", NAS, 20}, {"dial", Streaming, 25}, {"zoneplayer", Speaker, 60},
	{"printer", Printer, 50}, {"scanner", Printer, 30}, {"digitalsecuritycamera", Camera, 60}, {"basic:1", IoT, 5},
	{"hue", Light, 30}, {"tvdevice", TV, 50}, {"remoteuidevice", TV, 30},
}

// Classify returns the most likely type for in.
func Classify(in Input) Result {
	score := map[string]int{}
	add := func(t string, w int) { score[t] += w }

	for _, h := range in.Hostnames {
		h = strings.ToLower(h)
		for _, r := range hostnameRules {
			if strings.Contains(h, r.needle) {
				add(r.typ, r.weight)
			}
		}
	}
	vendor := strings.ToLower(in.Vendor + " " + in.UPnPMaker)
	for _, r := range vendorRules {
		if strings.Contains(vendor, r.needle) {
			add(r.typ, r.weight)
			break // the first (most specific) vendor rule wins
		}
	}
	for _, s := range in.Services {
		for _, r := range serviceRules[strings.ToLower(s)] {
			add(r.typ, r.weight)
		}
	}
	model := strings.ToLower(strings.ReplaceAll(in.Model, " ", ""))
	modelSpaced := strings.ToLower(in.Model)
	for _, r := range modelRules {
		if strings.HasPrefix(model, strings.ReplaceAll(r.needle, " ", "")) || strings.HasPrefix(modelSpaced, r.needle) {
			add(r.typ, r.weight)
			break
		}
	}
	if t, ok := homeKitCategories[in.HomeKitCat]; ok {
		add(t, 60)
	}
	for _, u := range in.UPnPTypes {
		u = strings.ToLower(u)
		for _, r := range upnpRules {
			if strings.Contains(u, r.needle) {
				add(r.typ, r.weight)
			}
		}
	}
	for _, p := range in.OpenPorts {
		for _, r := range portRules[p] {
			add(r.typ, r.weight)
		}
	}
	os := GuessOS(in.DHCPVendor, in.DHCPParams)
	switch os {
	case "iOS":
		add(Phone, 35)
	case "iPadOS":
		add(Tablet, 35)
	case "macOS":
		add(Laptop, 25)
		add(Computer, 20)
	case "Windows":
		add(Computer, 30)
		add(Laptop, 20)
	case "Android":
		add(Phone, 35)
	case "Embedded Linux":
		add(IoT, 15)
	}
	if in.MACKind == "virtual" {
		add(Virtual, 40)
	}
	if in.MACKind == "random" {
		// Private Wi-Fi addresses are almost exclusively phones, tablets and laptops.
		add(Phone, 20)
		add(Laptop, 8)
	}

	best, bestScore := Unknown, 0
	for _, t := range All {
		if s := score[t]; s > bestScore {
			best, bestScore = t, s
		}
	}
	if in.IsGateway {
		best, bestScore = Router, max(bestScore, 90)
	}
	if in.IsSelf && best != NAS {
		best, bestScore = Server, max(bestScore, 90)
	}
	if bestScore < 15 {
		return Result{Type: Unknown, Confidence: bestScore, OS: os}
	}
	return Result{Type: best, Confidence: min(bestScore, 100), OS: os}
}

// GuessOS derives the operating system family from DHCP options 60 and 55.
// The rules are Netward's own, derived from public RFCs and observation.
func GuessOS(vendorClass, params string) string {
	vc := strings.ToLower(vendorClass)
	switch {
	case strings.HasPrefix(vc, "android-dhcp"):
		return "Android"
	case strings.HasPrefix(vc, "msft"):
		return "Windows"
	case strings.Contains(vc, "dhcpcd") && strings.Contains(vc, "linux"):
		return "Linux"
	case strings.HasPrefix(vc, "udhcp"):
		return "Embedded Linux"
	case strings.Contains(vc, "esp32"), strings.Contains(vc, "esp8266"):
		return "Embedded"
	}
	if strings.HasPrefix(params, "1,121,3,6,15,") || strings.HasPrefix(params, "1,3,6,15,119,252") {
		// Apple: macOS asks for option 95 (LDAP) in addition to the common set.
		for _, p := range strings.Split(params, ",") {
			if p == "95" {
				return "macOS"
			}
		}
		return "iOS"
	}
	if strings.HasPrefix(params, "1,3,6,15,31,33,43,44,46,47,119,121,249,252") ||
		strings.HasPrefix(params, "1,15,3,6,44,46,47,31,33,121,249,43") {
		return "Windows"
	}
	if strings.HasPrefix(params, "1,3,6,15,26,28,51,58,59") {
		return "Android"
	}
	return ""
}
