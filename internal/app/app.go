// Package app wires all components together and runs them.
package app

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Pyzyi/netzwart/internal/api"
	"github.com/Pyzyi/netzwart/internal/auth"
	"github.com/Pyzyi/netzwart/internal/config"
	"github.com/Pyzyi/netzwart/internal/egress"
	"github.com/Pyzyi/netzwart/internal/fritzbox"
	"github.com/Pyzyi/netzwart/internal/hub"
	"github.com/Pyzyi/netzwart/internal/internet"
	"github.com/Pyzyi/netzwart/internal/monitor"
	"github.com/Pyzyi/netzwart/internal/mqtt"
	"github.com/Pyzyi/netzwart/internal/netinfo"
	"github.com/Pyzyi/netzwart/internal/notify"
	"github.com/Pyzyi/netzwart/internal/persons"
	"github.com/Pyzyi/netzwart/internal/privdrop"
	"github.com/Pyzyi/netzwart/internal/scanner"
	"github.com/Pyzyi/netzwart/internal/security"
	"github.com/Pyzyi/netzwart/internal/software"
	"github.com/Pyzyi/netzwart/internal/store"
	"github.com/Pyzyi/netzwart/internal/update"
	"github.com/Pyzyi/netzwart/internal/wlan"
)

// DBPath returns the database file inside the data directory.
func DBPath(env config.Env) string { return filepath.Join(env.DataDir, "netzwart.db") }

// LoadSettings reads settings from the store on top of the defaults, so new
// fields added in later versions get their default values.
func LoadSettings(ctx context.Context, st *store.Store) (config.Settings, error) {
	s := config.Defaults()
	var raw json.RawMessage
	err := st.GetSetting(ctx, "settings", &raw)
	if errors.Is(err, store.ErrNotFound) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return config.Defaults(), nil
	}
	if s.Validate() != nil {
		// Keep the user's data but repair invalid values with defaults.
		d := config.Defaults()
		if s.ScanInterval < 30 || s.ScanInterval > 3600 {
			s.ScanInterval = d.ScanInterval
		}
		if s.OfflineAfter < 2*s.ScanInterval {
			s.OfflineAfter = max(d.OfflineAfter, 2*s.ScanInterval)
		}
	}
	return s, nil
}

// Options control Run.
type Options struct {
	Env     config.Env
	Priv    privdrop.Result
	Version string
	Static  fs.FS
	Log     *slog.Logger
}

// Run starts Netzwart and blocks until ctx ends or a fatal error occurs.
func Run(ctx context.Context, o Options) error {
	log := o.Log
	env := o.Env
	if err := privdrop.CheckDataDir(env.DataDir); err != nil {
		return err
	}
	st, err := store.Open(ctx, DBPath(env))
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer st.Close()
	for _, w := range st.Warnings() {
		log.Warn(w)
	}
	if err := handleResetFiles(ctx, env, st, log); err != nil {
		log.Error("reset trigger failed", "err", err)
	}
	settings, err := LoadSettings(ctx, st)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	h := hub.New()
	policy := egress.DefaultPolicy
	disp := notify.NewDispatcher(st, policy, log)
	disp.SetLocale(settings.Language, settings.Timezone)
	if err := disp.Reload(ctx); err != nil {
		return err
	}
	if env.Demo {
		if err := SeedDemoLang(ctx, st, env.DemoLang); err != nil {
			return fmt.Errorf("seed demo data: %w", err)
		}
	}
	mon, err := monitor.New(ctx, st, h, disp, settings, log)
	if err != nil {
		return err
	}
	var sc *scanner.Scanner
	if !env.Demo {
		sc = scanner.New(mon, h, o.Priv.Links, o.Priv.Pinger, env.Interfaces, settings, log)
		mon.SetProber(sc)
		mon.SetEnricher(sc)
	}
	gateways := func() []netip.Addr {
		routes, _ := netinfo.DefaultGateways()
		var out []netip.Addr
		for _, r := range routes {
			out = append(out, r.Gateway)
		}
		return out
	}
	inet := internet.New(st, h, mon, disp, o.Priv.Pinger, policy, gateways, log)
	inet.Configure(settings.InternetCheck && !env.Demo, settings.InternetTargets)
	if env.Demo {
		inet.SeedDemo(time.Now())
	}
	sec := security.New(st, mon, h, disp, log)
	if sc != nil {
		sec.SetDHCPProber(sc)
	}
	sec.SetDHCPCheck(settings.DHCPCheck)
	people, err := persons.New(ctx, st, mon, h)
	if err != nil {
		return err
	}
	bridge := mqtt.NewBridge(mon, h, policy, inet.Status, o.Version, log)
	bridge.SetPersons(people)
	bridge.Configure(settings.MQTT, settings.Language)
	fritz := fritzbox.NewPoller(st, h, policy.DialContext, func(ctx context.Context, hosts []fritzbox.Host, at time.Time) {
		rh := make([]monitor.RouterHost, 0, len(hosts))
		for _, x := range hosts {
			rh = append(rh, monitor.RouterHost{MAC: x.MAC.String(), IP: x.IP, Name: x.Name, Active: x.Active, WLAN: x.WLAN})
		}
		mon.ApplyRouterHosts(ctx, rh, at.UnixMilli())
	}, log)
	fritz.OnLine(inet.ApplyLine)
	wifi := wlan.New(mon)
	fritz.OnWLAN(func(_ context.Context, st fritzbox.WLANState, at time.Time) { wifi.Apply(st, at) })
	if env.Demo {
		wifi.SeedDemo(DemoWLANLang(env.DemoLang), time.Now())
	}
	fritz.OnFirmware(func(ctx context.Context, host string, fw fritzbox.Firmware) {
		// The router is the configured host if that is an address, else an
		// AVM gateway, else the only AVM router; never a guess.
		var id int64
		if ip, err := netip.ParseAddr(host); err == nil {
			if d, ok := mon.DeviceByIP(ip); ok {
				id = d.ID
			}
		}
		if id == 0 {
			var avm []store.Device
			for _, d := range mon.Devices() {
				if strings.Contains(strings.ToLower(d.Vendor+" "+d.VendorOfficial), "avm") && d.Type == "router" {
					avm = append(avm, d)
					if d.IsGateway {
						id = d.ID
					}
				}
			}
			if id == 0 && len(avm) == 1 {
				id = avm[0].ID
			}
		}
		if id == 0 {
			return
		}
		mon.AddSoftware(id, software.FromFritzBox(fw.Version, fw.UpgradeAvailable, fw.NewVersion))
	})
	mon.SetSoftwareHook(sec.TriggerSoftware)
	upd := update.New(policy.HTTPClient(15*time.Second, 0), o.Version)
	upd.Configure(settings.UpdateCheck && !env.Demo)
	if !env.Demo {
		fritz.Configure(ctx, settings.FritzBox)
	}

	var smu sync.Mutex
	current := settings
	getSettings := func() config.Settings {
		smu.Lock()
		defer smu.Unlock()
		return current
	}
	apply := func(ctx context.Context, s config.Settings) error {
		if err := s.Validate(); err != nil {
			return err
		}
		if err := st.SetSetting(ctx, "settings", s); err != nil {
			return err
		}
		smu.Lock()
		current = s
		smu.Unlock()
		mon.UpdateSettings(s)
		if sc != nil {
			sc.UpdateSettings(s)
		}
		disp.SetLocale(s.Language, s.Timezone)
		inet.Configure(s.InternetCheck && !env.Demo, s.InternetTargets)
		bridge.Configure(s.MQTT, s.Language)
		upd.Configure(s.UpdateCheck && !env.Demo)
		sec.SetDHCPCheck(s.DHCPCheck)
		if !env.Demo {
			fritz.Configure(ctx, s.FritzBox)
		}
		h.Publish("settings", s.Public())
		return nil
	}

	srv := api.New(api.Deps{
		Store: st, Monitor: mon, Scanner: sc, Internet: inet, Security: sec, Notify: disp, MQTT: bridge, FritzBox: fritz, Hub: h,
		Update: upd, WLAN: wifi, Persons: people,
		Env: env, Log: log, Version: o.Version, StartedAt: time.Now(), Static: o.Static,
		Settings: getSettings, ApplySettings: apply, LocalPrefixes: api.LocalPrefixes, RawSockets: len(o.Priv.Links) > 0,
	})
	httpSrv := &http.Server{
		Addr:              net.JoinHostPort(env.Listen, strconv.Itoa(env.Port)),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second, // the event stream lifts this per response
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelDebug),
	}
	ln, err := net.Listen("tcp", httpSrv.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", httpSrv.Addr, err)
	}

	var wg sync.WaitGroup
	start := func(name string, f func(context.Context) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := f(ctx); err != nil && ctx.Err() == nil {
				log.Error("component stopped", "component", name, "err", err)
				cancel(fmt.Errorf("%s: %w", name, err))
			}
		}()
	}
	if !env.Demo {
		start("monitor", func(ctx context.Context) error { mon.Run(ctx); return nil })
		start("scanner", sc.Run)
		start("watchdog", func(ctx context.Context) error { return watchdog(ctx, st, mon, sc, getSettings) })
	}
	start("notify", func(ctx context.Context) error { disp.Run(ctx); return nil })
	if !env.Demo {
		start("internet", func(ctx context.Context) error { inet.Run(ctx); return nil })
	}
	start("security", func(ctx context.Context) error {
		sec.Schedule(ctx, func() int { return getSettings().SecurityScanDays })
		return nil
	})
	start("software", func(ctx context.Context) error { sec.WatchSoftware(ctx); return nil })
	start("update", func(ctx context.Context) error { upd.Run(ctx); return nil })
	start("mqtt", func(ctx context.Context) error { bridge.Run(ctx); return nil })
	start("fritzbox", func(ctx context.Context) error { fritz.Run(ctx); return nil })
	start("maintenance", func(ctx context.Context) error { maintenance(ctx, env, st, mon, log); return nil })

	scheme := "http"
	errc := make(chan error, 1)
	go func() {
		var err error
		if env.TLSCert != "" {
			scheme = "https"
			httpSrv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
			err = httpSrv.ServeTLS(ln, env.TLSCert, env.TLSKey)
		} else {
			err = httpSrv.Serve(ln)
		}
		if !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()
	mode := "raw"
	if len(o.Priv.Links) == 0 {
		mode = "fallback"
	}
	if env.Demo {
		mode = "demo"
	}
	log.Info("Netzwart is running", "version", o.Version, "url", fmt.Sprintf("%s://<server-ip>:%d", scheme, env.Port),
		"mode", mode, "uid", os.Getuid(), "capabilities", o.Priv.CapEff)

	var runErr error
	select {
	case <-ctx.Done():
		runErr = context.Cause(ctx)
		if errors.Is(runErr, context.Canceled) {
			runErr = nil
		}
	case err := <-errc:
		runErr = err
	}
	shutdownCtx, c2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer c2()
	_ = httpSrv.Shutdown(shutdownCtx)
	cancel(nil)
	wg.Wait()
	return runErr
}

// watchdog ends the process when scanning or the database is stuck, so the
// container restart policy can recover (Docker never restarts "unhealthy").
func watchdog(ctx context.Context, st *store.Store, mon *monitor.Monitor, sc *scanner.Scanner, settings func() config.Settings) error {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	started := time.Now()
	dbFailures := 0
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
		if err := st.SetSetting(ctx, "watchdog", time.Now().UnixMilli()); err != nil {
			dbFailures++
			if dbFailures >= 3 {
				return fmt.Errorf("database writes failing: %w", err)
			}
		} else {
			dbFailures = 0
		}
		limit := 3*time.Duration(settings().ScanInterval)*time.Second + 5*time.Minute
		last := mon.LastSweep()
		if last.IsZero() {
			last = started
		}
		swept, since := sc.Health()
		if !since.IsZero() && time.Since(since) > limit {
			return fmt.Errorf("scan stuck for %s", time.Since(since).Round(time.Second))
		}
		// Only when something is actually swept: a skipped (too large) or
		// missing interface must not cause a restart loop.
		if swept > 0 && since.IsZero() && time.Since(last) > limit {
			return fmt.Errorf("no completed scan for %s", time.Since(last).Round(time.Second))
		}
		if swept == 0 {
			started = time.Now() // start counting once an interface appears
		}
	}
}

// maintenance runs daily: retention, expiry and a consistent backup.
func maintenance(ctx context.Context, env config.Env, st *store.Store, mon *monitor.Monitor, log *slog.Logger) {
	t := time.NewTimer(10 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		mon.Maintain(ctx)
		if err := backup(ctx, env, st); err != nil {
			log.Warn("backup failed", "err", err)
		}
		t.Reset(24 * time.Hour)
	}
}

func backup(ctx context.Context, env config.Env, st *store.Store) error {
	dir := filepath.Join(env.DataDir, "backup")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	name := filepath.Join(dir, "netzwart-"+time.Now().Format("2006-01-02")+".db")
	_ = os.Remove(name)
	if err := st.Backup(ctx, name); err != nil {
		return err
	}
	entries, _ := os.ReadDir(dir)
	var files []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "netzwart-") && strings.HasSuffix(e.Name(), ".db") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	for len(files) > 3 {
		_ = os.Remove(filepath.Join(dir, files[0]))
		files = files[1:]
	}
	// The copy made before a schema update is the way back to the previous
	// version; after 30 days nobody goes back, and it holds old secrets.
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "pre-update-schema-") {
			if fi, err := e.Info(); err == nil && time.Since(fi.ModTime()) > 30*24*time.Hour {
				_ = os.Remove(filepath.Join(dir, e.Name()))
			}
		}
	}
	return nil
}

// handleResetFiles implements the file triggers for users without shell
// access: a file "reset-password" or "factory-reset" in the data directory.
func handleResetFiles(ctx context.Context, env config.Env, st *store.Store, log *slog.Logger) error {
	if _, err := os.Stat(filepath.Join(env.DataDir, "factory-reset")); err == nil {
		if err := st.FactoryReset(ctx); err != nil {
			return err
		}
		_ = os.Remove(filepath.Join(env.DataDir, "factory-reset"))
		log.Warn("FACTORY RESET performed: accounts, sessions, channels and settings were removed. Open the web UI to set up again.")
	}
	trigger := filepath.Join(env.DataDir, "reset-password")
	if _, err := os.Stat(trigger); err != nil {
		return nil
	}
	defer os.Remove(trigger)
	u, err := st.FirstUser(ctx)
	if errors.Is(err, store.ErrNotFound) {
		log.Warn("reset-password: no account exists yet; open the web UI to set one up")
		return nil
	}
	if err != nil {
		return err
	}
	pw := RandomPassword()
	hash, err := auth.HashPassword(ctx, pw)
	if err != nil {
		return err
	}
	if err := st.SetPassword(ctx, u.ID, hash, 0); err != nil {
		return err
	}
	log.Warn("PASSWORD RESET via trigger file", "user", u.Username, "new_password", pw,
		"hint", "log in and change it under Settings > Security")
	return nil
}

// RandomPassword returns a readable one-time password (16 symbols from an
// alphabet of 55, about 92 bits).
func RandomPassword() string {
	const alphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	var b strings.Builder
	for i := 0; i < 16; i++ {
		if i > 0 && i%4 == 0 {
			b.WriteByte('-')
		}
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			panic(err) // crypto/rand does not fail on Linux
		}
		b.WriteByte(alphabet[n.Int64()])
	}
	return b.String()
}
