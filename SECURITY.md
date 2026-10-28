# Sicherheitsrichtlinie

> **English:** Please report vulnerabilities privately via
> [Report a vulnerability](https://github.com/Pyzyi/Netward/security/advisories/new), not as a public issue.
> You will get an answer within 7 days. Security fixes are provided for the latest release.

## Sicherheitslücke melden

Bitte **nicht** als öffentliches Issue. Nutze stattdessen
[„Report a vulnerability“](https://github.com/Pyzyi/Netward/security/advisories/new) (GitHub Private Vulnerability Reporting).
Du bekommst innerhalb von 7 Tagen eine Antwort. Ein Fix erscheint, sobald er geprüft ist, zusammen mit einem Advisory,
in dem du genannt wirst, wenn du möchtest.

## Unterstützte Versionen

Sicherheitsupdates gibt es für die jeweils neueste Version (`latest` und das neueste `vX.Y`-Tag).

## Was Netward schützt

Die vollständige Analyse steht im [Bedrohungsmodell](docs/03-bedrohungsmodell.md) (STRIDE, 29 Bedrohungen mit
Gegenmaßnahme und Nachweis). Kurzfassung:

- **Zugang:** Kein Standardpasswort. Die Ersteinrichtung ist nur 30 Minuten nach dem Start und nur aus direkt
  angeschlossenen Netzen möglich, optional zusätzlich mit Token. Passwörter mit Argon2id, Anmeldebremse pro
  IP-Adresse, Sitzungen als `HttpOnly`/`SameSite=Strict`-Cookie, serverseitig widerrufbar.
- **Browser-Angriffe:** CSRF-Token und Origin-Prüfung für alle ändernden Anfragen, Host-Allowlist gegen DNS-Rebinding,
  strikte Content-Security-Policy ohne Inline-Skripte, kein `innerHTML` mit Gerätedaten.
- **Daten aus dem Netz** (Hostnamen, mDNS, UPnP, DHCP) gelten als feindlich: Parser sind gefuzzt, Texte werden
  bereinigt und je Ausgabekanal passend kodiert (HTML, Telegram, Discord, Slack, E-Mail-Header, CSV, MQTT).
- **Ausgehende Verbindungen** (Webhooks, MQTT, UPnP-Beschreibungen, FRITZ!Box) laufen über eine zentrale Richtlinie,
  die Loopback, Link-Local und Multicast verbietet. So kann niemand über Netward Dienste auf dem Host selbst erreichen.
- **Container:** Läuft als UID 10001 ohne Linux-Capabilities, schreibgeschütztes Dateisystem, Image ohne Shell.
  Images sind für amd64/arm64 gebaut, mit SBOM und Provenance versehen und mit Sigstore cosign signiert.

## Image prüfen

```bash
cosign verify ghcr.io/pyzyi/netward:latest \
  --certificate-identity-regexp '^https://github\.com/Pyzyi/Netward/\.github/workflows/ci\.yml@refs/(heads/main|tags/v[0-9.]+)$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

## Bekannte Grenzen

Siehe „Restrisiken“ im [Bedrohungsmodell](docs/03-bedrohungsmodell.md#4-restrisiken-akzeptiert), insbesondere:
HTTP im LAN ist Standard (HTTPS über `NETWARD_TLS_CERT`/`_KEY` oder Reverse-Proxy), und das Host-Netzwerk ist für
Layer-2-Erkennung technisch nötig.
