# 05 · UX- und Design-Konzept

> Rolle: Product Designer · Status: **überarbeitet nach Plan-Review 1** (siehe [07](07-review-protokoll.md))

## 1. Gestaltungsprinzipien

1. **Antwort zuerst.** Jede Seite beantwortet zuerst die Frage, mit der man sie öffnet
   („Ist alles in Ordnung?“, „Wer ist das?“). Details folgen auf Nachfrage.
2. **Ruhig statt laut.** Eine Akzentfarbe, neutrale Flächen. Farbe nur für Status und Aktion.
   Keine Verläufe, kein Glas-Effekt, keine Emojis, keine Deko-Illustrationen.
3. **Dichte wie ein Werkzeug.** Listen sind kompakt und scannbar wie in Fing:
   Icon, Name, Hersteller/IP, Status. Technische Werte (IP, MAC) stehen in Monospace mit tabellarischen Ziffern.
4. **Sprache wie ein Mensch.** „Neues Gerät im Netz“ statt „Event: DEVICE_NEW“.
   Relative Zeiten („vor 3 Min.“), Tooltip mit exaktem Zeitpunkt.
5. **Sicher ohne Angst.** Sicherheitsbefunde nüchtern formuliert, immer mit konkretem nächsten Schritt.

## 2. Design-Tokens

### Farben

| Token | Hell | Dunkel | Verwendung |
|---|---|---|---|
| `--bg` | `#F4F5F7` | `#0D1015` | App-Hintergrund |
| `--surface` | `#FFFFFF` | `#151920` | Karten, Listen |
| `--surface-2` | `#F0F2F5` | `#1B2029` | Hover, Eingabefelder |
| `--border` | `#E2E5EA` | `#262C37` | Trennlinien |
| `--text` | `#121519` | `#E7EAEE` | Primärtext |
| `--text-2` | `#5B6370` | `#98A1AE` | Sekundärtext |
| `--accent` | `#1662D8` | `#5A93F0` | Aktionen, Links, Auswahl |
| `--ok` | `#0F7A41` | `#3CC27A` | Online |
| `--warn` | `#8F5600` | `#E6A23C` | Neu/unbekannt, Hinweise |
| `--danger` | `#C8312F` | `#F06B68` | Kritische Befunde, Löschen |
| `--off` | `#9AA1AC` | `#5C6573` | Offline-**Punkte** (Grafik); Offline-Text nutzt `--text-2` |

Text-/Hintergrund-Kombinationen erreichen ≥ 4,5:1, Statusgrafiken ≥ 3:1 neben ihrem Textlabel.
Im Dunkelmodus tragen Akzent-Buttons dunklen Text (`#0D1015` auf `#5A93F0` ≈ 6,2:1).
Ein Skript in CI prüft alle Token-Paare.

### Typografie

- **IBM Plex Sans** (Variable, lokal eingebunden) für UI. **IBM Plex Mono** für IP/MAC/Ports.
- Skala: 12 / 13 / 14 (Basis) / 16 / 20 / 28 px; Zeilenhöhe 1,45; `font-variant-numeric: tabular-nums` für Zahlen.

### Raster und Formen

- 4-px-Raster; Abstände 4/8/12/16/24/32/48.
- Radien: 6 px (Steuerelemente), 10 px (Karten), rund (Geräte-Icons, Status-Punkte).
- Schatten nur für schwebende Elemente (Panels, Menüs). Karten haben Rahmen statt Schatten.
- Bewegung: 120–180 ms, `cubic-bezier(.2,.8,.2,1)`; bei `prefers-reduced-motion` deaktiviert.

### Geräte-Icons

Kreis 36–40 px mit leicht getönter Fläche je Kategorie (Netzwerk, Computer, Mobil, Medien,
Smart Home, Peripherie, Unbekannt) und Lucide-Linien-Icon in der Kategoriefarbe. Der Status-Punkt
sitzt unten rechts am Icon, wie in Fing, mit Rahmen in Flächenfarbe.

## 3. Informationsarchitektur

```
Übersicht ─ Geräte ─ Aktivität ─ Sicherheit ─ Einstellungen
                │
                └─ Gerätedetail (Seitenpanel Desktop / Vollbild Mobil)
```

- **Desktop (≥ 1024 px):** Linke Navigationsleiste (220 px), oben Netzname und Scan-Status.
  Das Gerätedetail öffnet als rechtes Panel (440 px), die Liste bleibt sichtbar.
- **Tablet (640–1023 px):** Eingeklappte Icon-Navigation.
- **Mobil (< 640 px):** Untere Tab-Leiste (wie Fing-App), Detail als Vollbild mit Zurück-Geste/Knopf.

## 4. Screens

### Ersteinrichtung (2 Schritte)
1. „Willkommen bei Netzwart“: Benutzername + Passwort (≥ 10 Zeichen, Stärkeanzeige über Länge/Zeichenklassen/Sperrliste, ohne große Bibliothek).
2. „Dein Netzwerk“: erkannte Interfaces/Subnetze vorausgewählt, Sprache. **Scan starten**.
Danach füllt sich die Übersicht live, ohne leeren Zwischenzustand.

### Übersicht
- **Kopfbereich:** Netzname (editierbar, z. B. „Zuhause“), Subnetz · Gateway, Button „Jetzt scannen“ mit Fortschritt.
- **Kennzahlen (4 Kacheln):** Online · Offline · Neu · Internet (Latenz, Status).
- **„Braucht Aufmerksamkeit“:** neue unbekannte Geräte mit Direktaktionen *Bekannt* / *Details*.
  Leerer Zustand: „Alles bekannt. Neue Geräte erscheinen hier.“
- **Letzte Aktivität:** die letzten 8 Ereignisse, Link zu Aktivität.
- **Nach Typ:** kompakte Verteilung (Telefone 6, Computer 4, …) als Filter-Einstieg.

### Geräte
- Suchfeld (`/`), Filter-Chips: Alle · Online · Offline · Neu · Favoriten; Gruppierung: keine / Typ / Ort.
- Zeile: Icon+Status · **Name** · Hersteller/Modell · IP (mono) · zuletzt gesehen/„online“ · Favoritenstern.
- Sortierung: Status, Name, IP (numerisch), zuletzt gesehen.

### Gerätedetail
- Kopf: großes Icon, Name (inline editierbar), Status-Pill, Typ-Auswahl.
- Aktionen: *Bekannt markieren*, *Beobachten* (Benachrichtigung bei online/offline), *Favorit*,
  *Aufwecken (WoL)*, *Ping*, *Ports scannen*, Menü: *Vergessen*.
- Abschnitte: **Anwesenheit** (Balken-Zeitleiste 24 h / 7 d / 30 d + Online-Anteil), **Netzwerk**
  (IP, MAC kopierbar, Hersteller, Interface, IP-Historie), **Erkannt als** (alle Namen mit Quelle,
  Dienste aus mDNS/SSDP, DHCP-Fingerprint/OS), **Offene Ports**, **Notizen & Ort**.

### Aktivität
Chronologische Liste nach Tagen gruppiert, Filter nach Art. Jede Zeile: Icon, Satz („**iPhone von Anna** ist wieder online“), Zeit.

### Sicherheit
Ergebnisliste nach Schweregrad (Kritisch / Warnung / Hinweis) mit Erklärung „Warum ist das wichtig?“
und „Was tun?“. Kopf: letzter Check, Button „Jetzt prüfen“. Kein Gamification-Score.

### Einstellungen
Abschnitte: Allgemein · Scannen · Benachrichtigungen · Home Assistant · Sicherheit & Konto · Daten · Über.

## 5. Mikrotexte (Auswahl)

| Situation | Text |
|---|---|
| Neues Gerät | „Neues Gerät im Netz: **{name}** ({vendor}, {ip})“ |
| Gerät offline | „**{name}** ist offline“ |
| Leere Geräteliste nach Filter | „Keine Geräte für „{query}“. Filter zurücksetzen?“ |
| Fallback-Modus | „Eingeschränkter Scanmodus: Dem Container fehlt die Berechtigung `NET_RAW`. Geräte werden langsamer erkannt.“ |
| HTTP-Hinweis | „Verbindung unverschlüsselt. Im Heimnetz meist in Ordnung, für Zugriff von außen bitte HTTPS verwenden.“ |

## 6. Anti-Muster (verbindlich vermeiden)

- Hero-Banner, Marketing-Sprache, „Willkommen zurück 👋“
- Lila-Blau-Verläufe, Glassmorphism, Neon-Glow, übermäßige Schatten
- Karten in Karten, Icons ohne Funktion, Platzhaltertexte
- Spinner für alles: stattdessen Skeletons bei Listen, Inline-Fortschritt bei Aktionen
- Modale Dialoge für Dinge, die inline gehen
