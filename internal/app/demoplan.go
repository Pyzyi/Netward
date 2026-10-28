package app

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"time"

	"github.com/Pyzyi/netzwart/internal/store"
)

// demoFloorPlan draws the sample flat for the Wi-Fi map (docs/10, F4): a
// hallway across the middle, three rooms above, three below.
func demoFloorPlan() []byte {
	const w, h = 1200, 760
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	fill := func(x0, y0, x1, y1 int, c color.Color) {
		draw.Draw(img, image.Rect(x0, y0, x1, y1), image.NewUniform(c), image.Point{}, draw.Src)
	}
	fill(40, 40, 1160, 720, color.RGBA{0xf4, 0xf3, 0xef, 0xff})  // floor
	fill(40, 330, 1160, 430, color.RGBA{0xec, 0xea, 0xe4, 0xff}) // hallway
	wall := color.RGBA{0x3a, 0x3f, 0x47, 0xff}
	const t = 10
	hwall := func(y, x0, x1 int, doors ...int) { // doors: x of 80 px openings
		for x := x0; x < x1; {
			end := x1
			for _, d := range doors {
				if d >= x && d < end {
					end = d
				}
			}
			fill(x, y-t/2, end, y+t/2, wall)
			x = end + 80
		}
	}
	vwall := func(x, y0, y1 int) { fill(x-t/2, y0, x+t/2, y1, wall) }
	hwall(40, 40, 1160)
	hwall(720, 40, 1160)
	vwall(40, 40, 720)
	vwall(1160, 40, 720)
	fill(30, 360, 50, 400, color.White) // front door
	hwall(330, 40, 1160, 250, 600, 900)
	hwall(430, 40, 1160, 300, 650, 950)
	vwall(360, 40, 330)
	vwall(720, 40, 330)
	vwall(600, 430, 720)
	vwall(820, 430, 720)
	// Windows: light gaps in the outer wall.
	glass := color.RGBA{0xbf, 0xd4, 0xe6, 0xff}
	for _, x := range []int{120, 460, 860, 1000} {
		fill(x, 36, x+120, 44, glass)
	}
	for _, x := range []int{140, 320, 900} {
		fill(x, 716, x+140, 724, glass)
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// seedWifiMap adds the sample flat with measurements: weak in the kids'
// room (on 2.4 GHz), jumpy in the bedroom.
func seedWifiMap(ctx context.Context, st *store.Store, lang string, phone int64, now time.Time) {
	p, err := st.AddFloorPlan(ctx, store.FloorPlan{Name: demoText(lang, "Wohnung"), Mime: "image/png", Width: 1200, Height: 760,
		CreatedAt: now.Add(-2 * time.Hour).UnixMilli()}, demoFloorPlan())
	if err != nil {
		return
	}
	pts := []struct {
		room            string
		x, y, down, lat float64
		jit             float64
		band            string
		dbm             int
	}{
		{"Wohnzimmer", 0.20, 0.75, 312, 3, 1, "5", -48},
		{"Wohnzimmer", 0.38, 0.80, 286, 3, 1, "5", -52},
		{"Küche", 0.15, 0.24, 238, 4, 1, "5", -58},
		{"Flur", 0.45, 0.50, 205, 4, 2, "5", -56},
		{"Arbeitszimmer", 0.45, 0.22, 176, 5, 2, "5", -61},
		{"Flur", 0.75, 0.50, 151, 6, 3, "5", -64},
		{"Bad", 0.59, 0.80, 142, 6, 3, "5", -66},
		{"Schlafzimmer", 0.83, 0.80, 158, 41, 37, "5", -67},
		{"Kinderzimmer", 0.72, 0.30, 71, 9, 6, "5", -73},
		{"Kinderzimmer", 0.88, 0.22, 38, 14, 9, "2.4", -79},
	}
	for i, x := range pts {
		_, _ = st.AddWifiPoint(ctx, store.WifiPoint{PlanID: p.ID, X: x.x, Y: x.y, Room: demoText(lang, x.room), DownMbps: x.down,
			UpMbps: float64(int(x.down*0.45*10)) / 10, LatencyMs: x.lat, JitterMs: x.jit, DeviceID: phone, Band: x.band,
			SignalDBm: x.dbm, AP: "FRITZ!Box 7590 AX", TS: now.Add(time.Duration(i-len(pts)) * 4 * time.Minute).UnixMilli()})
	}
}
