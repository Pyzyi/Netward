# 05 · UX- und Design-Konzept

> Rolle: Product Designer · Status: **Redesign 2 „Leitstand“** (Abschnitt 0 ersetzt die visuellen Festlegungen
> der Abschnitte 2–4; Prinzipien, Informationsarchitektur und Mikrotexte gelten weiter)

## 0. Redesign 2 · Designsprache „Leitstand“

### Warum
Die erste Fassung war sauber, aber austauschbar: weiße Karten mit 1-px-Rahmen, linke Icon-Navigation,
Kennzahl-Kacheln mit Farbpunkt, Pillen-Badges, SaaS-Blau, Lucide-Icons. Das ist der Baukasten fast jeder
generierten Admin-Oberfläche. Nutzerfeedback: „sieht aus wie die nächste Vibecoding-App“.

### Leitidee
Netzwart ist ein **Messinstrument für das eigene Netz**, kein SaaS-Dashboard. Vorbild ist ein Leitstand:
dunkler Kopf mit Live-Zustand, darunter ruhiges „Papier“ mit dichten, tabellarischen Listen. Farbe bedeutet
ausschließlich Zustand. Aktionen sind tintenschwarz, nicht blau.

### Erkennungszeichen (nur Netzwart hat das)
1. **Netzring.** Die Übersicht zeigt das Netz als Ring: Router in der Mitte, jedes Gerät ein Punkt, nach
   Kategorie gruppiert. Online gefüllt, offline hohl, neu mit Halo. Klick öffnet das Gerät. Live aktualisiert.
2. **Herzschlag-Streifen.** Jede Gerätezeile zeigt die letzten 24 Stunden als 48 Takte. Man sieht auf einen
   Blick, wer immer da ist, wer abends kommt, wer seit Tagen fehlt. Das kann weder Fing noch NetAlertX.
3. **Tintenkopf.** Ein durchgehendes dunkles Band trägt Netzname, Subnetz, Navigation und den Live-Puls
   des Scanners. Auf der Übersicht wird es zur Bühne für Ring und Kennzahlen.

### Regeln
| Thema | Regel |
|---|---|
| Fläche | Hintergrund warmes Papier `#F4F2EC`, nur schwebende Elemente (Detailpanel, Dialoge, Menüs) sind weiße Blätter. **Keine Karten-Raster.** Abschnitte trennen Haarlinien und Abstand. |
| Farbe | Tinte `#0F1B2D` für Kopf, Primäraktionen, Überschriften. Status: Signalgrün (online), Bernstein (neu/Hinweis), Ziegelrot (kritisch). Kein Akzentblau, keine Verläufe (einzige Ausnahme: der Halo neuer Geräte). |
| Schrift | **Schibsted Grotesk** (UI, Zahlen in Schwer 800) und **JetBrains Mono** (IP, MAC, Ports, Uhrzeiten, kleine Rubriken in Versalien). Große Zahlen statt Kacheln. |
| Icons | **Phosphor**: Duotone für Gerätetypen in quadratischen Kacheln (Radius 10), Regular für Bedienelemente. Keine Icons ohne Funktion. |
| Formen | Radien 4 px (Steuerelemente), 8 px (Blätter), 10 px (Geräte-Kacheln). Status-Pillen entfallen zugunsten von Text in Statusfarbe mit Punkt. |
| Dichte | Gerätezeile 52 px, Listen ohne Rahmen, Hover als Papier-Tönung. |
| Bewegung | Nur Bedeutung: Scan-Puls im Kopf, Halo neuer Geräte, Einblenden des Panels. `prefers-reduced-motion` schaltet alles ab. |
| Dunkelmodus | Kopf und Fläche verschmelzen zu Nachtblau; Primäraktionen werden hell mit dunkler Schrift. |

### Seiten
- **Übersicht:** Tintenbühne mit großer Zahl „24 von 28 online“, Neu/Offline/Internet als Zeile darunter,
  rechts der Netzring. Auf Papier: „Braucht Aufmerksamkeit“, „Letzte Aktivität“ (Zeitachse mit Uhrzeit in Mono),
  Sicherheit, Gerätetypen als ein gestapelter Balken statt 18 Zeilen.
- **Geräte:** Register mit Suche, Filter als Textreiter mit Zählern, Zeilen mit Kachel · Name/Modell · IP ·
  Herzschlag · Status.
- **Gerätedetail:** weißes Blatt rechts; Kopf mit großer Kachel, Name, Statuszeile; Aktionen als Werkzeugleiste.
- **Aktivität:** Tagesrubriken, Uhrzeit-Spalte in Mono, Schweregrad als feine Linie.
- **Sicherheit:** Prüfbericht: Schweregrad in Versalien links, Befund, Gerät, aufklappbar „Warum / Was tun“.
- **Anmeldung/Einrichtung:** geteilt: links Tintenfläche mit Ring-Grafik, rechts Formular auf Papier.

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

Die verbindlichen Tokens stehen in [`web/src/app.css`](../web/src/app.css) und folgen Abschnitt 0
(Papier, Tinte, Signalgrün, Bernstein, Ziegelrot; Schibsted Grotesk und JetBrains Mono; Radien 4/8/10 px).
Typografie-Skala: 11 (Mono-Rubriken), 13, 14 (Basis), 16, 20, 30 px sowie Displaygrößen für Kennzahlen;
Gewichte 400/500 (Mono)/600/800. Alle Text- und Steuerelement-Kontraste prüft `npm run contrast` in CI
(hell und dunkel, ≥ 4,5:1 für Text, ≥ 3:1 für Steuerelemente).

## 3. Informationsarchitektur

```
Übersicht ─ Geräte ─ Aktivität ─ Sicherheit ─ Einstellungen
                │
                └─ Gerätedetail (Seitenpanel Desktop / Vollbild Mobil)
```

- **Desktop (> 860 px):** Tintenkopf mit Logo, Netzname, Textreitern, Scan-Puls und Abmelden; die Navigation
  steht auf jeder Seite an derselben Stelle. Das Gerätedetail öffnet als Blatt rechts (470 px), die Liste bleibt sichtbar;
  unter 1100 px als modales Blatt.
- **Mobil (≤ 860 px):** Untere Tab-Leiste, Detail als Vollbild. Auf der Übersicht steht die Antwort zuerst
  (Kennzahlen, „Braucht Aufmerksamkeit“); der Netzring erscheint erst, wenn genug Platz für ihn ist.

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
