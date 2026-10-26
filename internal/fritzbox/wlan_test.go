package fritzbox

import (
	"os"
	"testing"
)

func TestParseMeshList(t *testing.T) {
	data, err := os.ReadFile("testdata/meshlist.json")
	if err != nil {
		t.Fatal(err)
	}
	st, err := ParseMeshList(data)
	if err != nil {
		t.Fatal(err)
	}
	if st.Source != "mesh" || len(st.APs) != 2 || len(st.Clients) != 2 {
		t.Fatalf("unexpected state %+v", st)
	}
	box, rep := st.APs[0], st.APs[1]
	if !box.Master || box.MAC != "3c:a6:2f:5e:11:01" || len(box.Bands) != 2 || box.Backhaul != nil {
		t.Fatalf("box %+v", box)
	}
	if rep.Master || rep.Name != "Repeater OG" || rep.Backhaul == nil || rep.Backhaul.Type != "wlan" ||
		rep.Backhaul.Band != "5" || rep.Backhaul.SignalDBm != -77 || rep.Backhaul.Rate != 86 {
		t.Fatalf("repeater %+v backhaul %+v", rep, rep.Backhaul)
	}
	phone, laptop := st.Clients[0], st.Clients[1]
	if phone.MAC != "aa:bb:cc:00:00:01" || phone.APMAC != box.MAC || phone.Band != "5" || phone.SignalDBm != -58 ||
		phone.RateRx != 864 || phone.MaxRate != 1201 {
		t.Fatalf("phone %+v", phone)
	}
	if laptop.APMAC != rep.MAC || laptop.Band != "2.4" || laptop.SignalDBm != -79 || !laptop.Can5GHz || laptop.Channel != 11 {
		t.Fatalf("laptop %+v", laptop)
	}
}

func TestRCPIAndBand(t *testing.T) {
	for rcpi, want := range map[int]int{255: 0, 0: 0, 220: 0, 104: -58, 62: -79} {
		if got := rcpiDBm(rcpi); got != want {
			t.Errorf("rcpiDBm(%d) = %d, want %d", rcpi, got, want)
		}
	}
	if bandOf("AP:6G:0", 37) != "6" || bandOf("", 11) != "2.4" || bandOf("", 100) != "5" || bandOf("x", 0) != "" {
		t.Fatal("band detection")
	}
}

func FuzzParseMeshList(f *testing.F) {
	if data, err := os.ReadFile("testdata/meshlist.json"); err == nil {
		f.Add(data)
	}
	f.Add([]byte(`{"nodes":[{"uid":"a","node_interfaces":[{"uid":"b","node_links":[{"node_interface_1_uid":"b","node_interface_2_uid":"b","state":"CONNECTED","type":"WLAN"}]}]}]}`))
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = ParseMeshList(data) })
}
