# 07 · Review-Protokoll

Dieses Dokument hält alle Reviews fest: wer geprüft hat, was gefunden wurde und wie entschieden wurde.
Befunde werden nicht gelöscht, sondern mit Entscheidung und Nachweis versehen.

## Review 1 · Plan-Review (vor Implementierungsbeginn)

- **Gegenstand:** Dokumente 01–06 in der Erstfassung
- **Reviewer:** unabhängiger Senior-Engineer/Security-Architekt (eigener Agent, ohne Kenntnis der Entwurfsdiskussion)
- **Ergebnis:** 7 blockierende, 16 mittlere, 6 niedrige Befunde. Die Planung wurde überarbeitet, alle Dokumente sind angepasst.

| # | Schwere | Befund (Kurzfassung) | Entscheidung | Umsetzung |
|---|---|---|---|---|
| B1 | hoch, blockierend | DNS-Rebinding: „Origin == Host“ schützt nicht, weil der Angreifer beide kontrolliert. Setup/Login ohne CSRF-Schutz | **Angenommen.** Host-Allowlist (IP-Literale, `localhost`, einteilige Namen, lokale Suffixe, `NETZWART_ALLOWED_HOSTS`) für *jeden* Request, Origin-Prüfung gegen die Allowlist, unsichere Requests ohne `Origin` werden abgelehnt, zusätzlich `http.CrossOriginProtection` | ADR-012, T-24 |
| B2 | hoch, blockierend | SSRF über Webhooks/Benachrichtigungen auf Dienste an `127.0.0.1` des Hosts (Host-Netzwerk!), Kette bis Root auf dem Host | **Angenommen.** Ein zentraler Egress-Dialer verbietet beim Verbindungsaufbau Loopback, Unspecified, Link-Local und Multicast. Gilt auch für Redirects und DNS. Kanaltests liefern nur den Statuscode | ADR-011, T-25 |
| B3 | hoch, blockierend | Setup-Schutz „private IP“ zu grob, Reset entfernt keine Angreifer-Konfiguration | **Angenommen.** Setup nur ≤ 30 min nach Start (danach Neustart nötig), Quelle muss in einem direkt angeschlossenen Subnetz liegen. Dazu `factory-reset` und Reset per Datei-Trigger | ADR-010 |
| B4 | hoch, blockierend | Privilegien-Abgabe nach ADR-004 scheitert an fehlendem `DAC_OVERRIDE`, an Per-Thread-Zustand und an `docker exec` als root | **Angenommen, Design ersetzt:** Raw-Sockets als root öffnen, dann `setresuid` **ohne** KEEPCAPS. Zur Laufzeit gilt CapEff = 0, ohne Re-Exec. Subkommandos wechseln vor jedem Datenzugriff auf PUID | ADR-004 (neu) |
| B5 | hoch, blockierend | MAC als alleinige Identität bricht bei MAC-NAT-Repeatern, Multi-IP-Geräten und mehreren Interfaces | **Angenommen.** Tabelle `device_addresses`; Proxy/Repeater-Erkennung (≥ 3 gleichzeitig aktive IPs), dahinter Identität (MAC, IP). `ip_changed` erst, wenn die alte IP länger als `offline_after` ungesehen ist | Datenmodell |
| B6 | hoch, blockierend | Presence zu naiv: schlafende Handys flackern, Downtime erscheint als „online“ | **Angenommen.** Schwellen je Gerätetyp (mobil 15 min), Weck-Proben vor „offline“ (ARP-Unicast, ICMP, TCP 62078, mDNS), passives mDNS, Heartbeat und Überwachungslücken, Timeout für das Startgate | §4.2 Architektur |
| B7 | hoch, blockierend | Geräte-/Benachrichtigungsflut durch zufällige MACs; rotierende private MACs | **Angenommen.** Aufnahme-Budget (20 neue/min, max. 4 000), Geräte nur aus ARP-Sichtungen im Subnetz, Ablauf unbestätigter privater MACs, Schalter „Bei privaten MACs benachrichtigen“, virtuelle Präfixe als „virtuell“, Geräte zusammenführen | T-26 |
| M1 | mittel | `/proc/net/arp` kennt keinen Nachbarstatus | **Angenommen.** rtnetlink `RTM_GETNEIGH`, nur `REACHABLE`, Fallback max. /24 | ADR-005 |
| M2 | mittel | ZimaOS-/Compose-Details | **Angenommen.** `x-casaos`, `container_name`, Auto-Erkennung nur über das Default-Route-Interface, CA-Bundle und tzdata im Image, macvlan-Einschränkung dokumentiert | Compose, README |
| M3 | mittel | Docker startet „unhealthy“ Container nicht neu | **Angenommen.** Interner Watchdog beendet den Prozess bei hängendem Scanner/DB, die Restart-Policy übernimmt | ADR-013 |
| M4 | mittel | Argon2id 64 MiB × 2 sprengt RAM-Ziel, Login-DoS | **Angenommen.** m = 19 MiB, t = 2, p = 1; 1 Slot mit Warteschlangen-Timeout (503) | T-02 |
| M5 | mittel | Injektion über Gerätenamen in Benachrichtigungen | **Angenommen.** Encoder je Kanal (Telegram Klartext, Discord `allowed_mentions`, Slack-Escaping, ntfy JSON, E-Mail text/plain + RFC 2047) mit Tests | T-27 |
| M6 | mittel | SSE: Verbindungslimit HTTP/1.1, Widerruf, Puffer | **Angenommen.** Stream schließt bei verborgenem Tab, Sitzungsprüfung pro Heartbeat, kein gzip, `X-Accel-Buffering: no` | ADR-007 |
| M7 | mittel | PWA braucht HTTPS; CSP konkretisieren; zxcvbn zu groß | **Angenommen.** F-28 umformuliert, CSP festgelegt, Stärkeanzeige ohne zxcvbn | F-28, T-05 |
| M8 | mittel | Kontrastwerte teils < 4,5:1 | **Angenommen.** Farben korrigiert, Kontrasttest in CI | 05 §2 |
| M9 | mittel | SSDP-Antworten sind spoofbar | **Angenommen.** Abruf über Egress-Dialer, nie an eigene Host-IPs, 1 Abruf/Gerät/6 h, nur GET ohne Cookies | T-07 |
| M10 | mittel | Falsch-Positive bei IP-Konflikten | **Angenommen.** Konflikt nur, wenn zwei MACs auf *denselben* Request antworten, und erst bei Bestätigung im nächsten Zyklus | §4.2 |
| M11 | mittel | Befunde nicht speicher- und quittierbar | **Angenommen.** Tabelle `security_findings` mit „akzeptiert“; Portprüfung nur auf Anforderung oder per Zeitplan (opt-in) | F-21 |
| M12 | mittel | SQLite-Betrieb (Backup, Schreiblast) | **Angenommen.** Nächtliches `VACUUM INTO`, `last_seen` gepuffert, Sitzungs-Touch max. 1×/min, `auto_vacuum=INCREMENTAL`, Hinweis zu NFS/SMB | ADR-002 |
| M13 | mittel | MQTT-Client unterschätzt | **Angenommen.** Vollwertige Komponente (QoS 1, LWT, retained, `homeassistant/status`, TLS, Reconnect) mit Test gegen Mosquitto | F-25 |
| M14 | mittel | Scope zu groß, erwartete Funktionen fehlen | **Teilweise.** Kanäle bleiben (jeder ist klein und einzeln getestet). Import, Latenzhistorie und Netzwerkkarte auf v1.1 verschoben. FRITZ!Box-TR-064 als Should aufgenommen. Zusammenführen und Quittieren aller Geräte aufgenommen; Internet-Ausfälle werden nach Wiederherstellung gemeldet | 01 |
| M15 | mittel | Lizenzen eingebetteter Daten | **Bestätigt unkritisch:** IEEE-Daten über `oui-data` (BSD-2), DHCP-Regeln selbst erstellt, keine Fingerbank-/Wireshark-Daten | NOTICE |
| M16 | mittel | AF_PACKET-Last bei NAS-Durchsatz | **Angenommen.** Getrennte Sockets (`ETH_P_ARP`, `ETH_P_IP` + enger BPF), `PACKET_IGNORE_OUTGOING`, Filter vor `bind` | ADR-005 |
| L1 | niedrig | Cookies sind nicht pro Port isoliert | **Angenommen.** Absolute Sitzungsdauer 30 d, `__Host-`-Präfix bei TLS, dokumentiert | T-03 |
| L2 | niedrig | Zeitzone | **Angenommen.** Browser-Zeitzone wird beim Setup gespeichert | – |
| L3 | niedrig | Reset per `docker exec` ist für Laien schwer | **Angenommen.** Datei `reset-password` im AppData-Ordner → Einmalpasswort im Log | F-14 |
| L4 | niedrig | Multicast-Interface, ICMP | **Angenommen.** `IP_MULTICAST_IF` je Interface, ICMP-Raw-Socket vor dem Drop, Gateway-Prüfung vor WAN-Prüfung | – |
| L5 | niedrig | Einige ISO-Ziele nicht messbar | **Angenommen.** Ziele präzisiert, Nichtabstreitbarkeit als „eingeschränkt“ markiert | 04 |
| L6 | niedrig | Testlücken | **Angenommen.** Testfälle ergänzt | 06 |

**Bestätigt korrekt (keine Änderung):** BPF über `x/net/bpf` ohne cgo, XXE-Sicherheit von `encoding/xml`,
`modernc.org/sqlite` auf amd64/arm64, das Image-Größenziel.

---

## Review 2 · Code-, Security- und UX-Review (nach Implementierung)

*(wird nach Abschluss der Implementierung ergänzt)*
