package classify

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		in   Input
		want string
	}{
		{"gateway always router", Input{Vendor: "Apple", IsGateway: true}, Router},
		{"fritzbox by vendor+host", Input{Vendor: "AVM", Hostnames: []string{"fritz.box"}}, Router},
		{"fritz repeater", Input{Vendor: "AVM", Hostnames: []string{"fritz-repeater"}}, AccessPoint},
		{"iphone by mdns model", Input{Vendor: "Apple", Model: "iPhone15,2"}, Phone},
		{"iphone by hostname", Input{Vendor: "Apple", Hostnames: []string{"Annas-iPhone"}}, Phone},
		{"random mac + ios dhcp", Input{MACKind: "random", DHCPParams: "1,121,3,6,15,108,114,119,252"}, Phone},
		{"macbook by dhcp", Input{Vendor: "Apple", DHCPParams: "1,121,3,6,15,108,114,119,252,95,44,46", Hostnames: []string{"MacBook-Pro"}}, Laptop},
		{"homepod", Input{Vendor: "Apple", Model: "AudioAccessory5,1", Services: []string{"_airplay._tcp", "_raop._tcp"}}, Speaker},
		{"chromecast", Input{Vendor: "Google", Services: []string{"_googlecast._tcp"}, Model: "Chromecast"}, Streaming},
		{"printer by services", Input{Vendor: "HP", Services: []string{"_ipp._tcp", "_pdl-datastream._tcp"}}, Printer},
		{"brother printer", Input{Vendor: "Brother", Hostnames: []string{"BRN3C2AF4123456"}}, Printer},
		{"synology", Input{Vendor: "Synology", OpenPorts: []int{5000, 5001, 445}}, NAS},
		{"sonos", Input{Vendor: "Sonos", OpenPorts: []int{1400}}, Speaker},
		{"samsung tv via upnp", Input{Vendor: "Samsung", UPnPTypes: []string{"urn:schemas-upnp-org:device:MediaRenderer:1", "urn:samsung.com:device:RemoteControlReceiver:1"}, Hostnames: []string{"Samsung-TV"}}, TV},
		{"shelly plug", Input{Vendor: "Espressif", Hostnames: []string{"shellyplug-s-ABC123"}}, Plug},
		{"esp generic", Input{Vendor: "Espressif"}, IoT},
		{"homekit lightbulb", Input{Vendor: "Espressif", HomeKitCat: "5"}, Light},
		{"hikvision camera", Input{Vendor: "Hikvision", OpenPorts: []int{554, 80}}, Camera},
		{"playstation", Input{Vendor: "Sony PlayStation"}, GameConsole},
		{"windows pc", Input{Vendor: "Intel", DHCPVendor: "MSFT 5.0", Hostnames: []string{"DESKTOP-4KJ2L"}}, Computer},
		{"android phone", Input{Vendor: "Samsung", DHCPVendor: "android-dhcp-14"}, Phone},
		{"self is server", Input{Vendor: "Intel", IsSelf: true}, Server},
		{"vm", Input{MACKind: "virtual"}, Virtual},
		{"nothing known", Input{}, Unknown},
		{"home assistant", Input{Vendor: "Raspberry Pi", OpenPorts: []int{8123}, Services: []string{"_home-assistant._tcp"}}, SmartHome},
	}
	for _, c := range cases {
		got := Classify(c.in)
		if got.Type != c.want {
			t.Errorf("%s: got %s (%d), want %s", c.name, got.Type, got.Confidence, c.want)
		}
		if !Valid(got.Type) {
			t.Errorf("%s: invalid type %q", c.name, got.Type)
		}
	}
}

func TestGuessOS(t *testing.T) {
	cases := map[[2]string]string{
		{"android-dhcp-13", ""}: "Android",
		{"MSFT 5.0", ""}:        "Windows",
		{"dhcpcd-9.4.1:Linux-6.1.0:aarch64:Raspberry", ""}: "Linux",
		{"udhcp 1.36.1", ""}:                               "Embedded Linux",
		{"", "1,121,3,6,15,108,114,119,252"}:               "iOS",
		{"", "1,121,3,6,15,108,114,119,252,95,44,46"}:      "macOS",
		{"", "1,3,6,15,31,33,43,44,46,47,119,121,249,252"}: "Windows",
		{"", "1,3,28"}: "",
	}
	for in, want := range cases {
		if got := GuessOS(in[0], in[1]); got != want {
			t.Errorf("GuessOS(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

func TestMobile(t *testing.T) {
	if !Mobile(Phone) || !Mobile(Watch) || Mobile(NAS) {
		t.Fatal("Mobile() wrong")
	}
}
