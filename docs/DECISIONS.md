# Entscheidungen

Wo Spezifikation und Vorlagen nicht eindeutig waren oder sich widersprochen haben.
Grundregel: Eine explizite Regel im Text gewinnt, ein „z. B.“ weicht den Bildern.

## Spielregeln

- **Levelkurve:** `100 * level^1.5` ist nur ein Beispiel und passt nicht zu den Bildern
  (8.940 XP wären Level 9). Die Kosten eines Levels sind `21,57 * level^1,822`, auf 10
  gerundet. Das ergibt genau Level 12 mit 1.440 / 2.000 XP. Die Kurve bleibt progressiv.
- **Taler pro Check-in:** Das Bild zeigt +5 Taler bei +10 XP, der Text sagt „ein Drittel“.
  Es gilt der Text: +3 Taler. Die Vorschau im Formular zeigt den echten Wert.
- **XP in den Bildern:** „Neue Idee einbringen“ zeigt +20 XP, das gibt es laut Regeln
  nicht (10/15/25). In den Demo-Daten ist das Habit mittel (+15).
- **XP-Anzeige:** Die Liste auf „Heute“ zeigt wie im Bild die Basis-XP ohne Serienbonus,
  „20 Min. lesen“ also +15. Der aktive Bonus steht im Habit-Detail.
- **Serien in Wochen:** Habits mit „n× pro Woche“ und eigenen Wochentagen zählen ihre Serie
  in Wochen, wie in den Bildern. Eine Woche zählt, wenn ihr Ziel erreicht ist. Die laufende
  Woche bricht die Serie nicht.
  - Serienbonus und Joker rechnen mit Wochen × 7 Tagen.
  - Serien-Abzeichen und „Längste Serie“ rechnen mit den Tagen mit Check-in
    (Wochen × Ziel pro Woche), sonst wäre die Monatsflamme in den Bildern schon erreicht.
    Abzeichen ab etwa „Ein Jahr“ sind damit praktisch tägliche Habits.
  - Bereichs-Abzeichen mit Zeitangabe („4 Wochen Serie“) rechnen mit der echten Dauer.
- **Vermeiden:** Ein Tag zählt und wird bezahlt, sobald er vorbei ist. Am ersten Tag steht
  die Serie auf 0. Ein nachgetragener Rückfall nimmt keine XP weg.
- **Serien-Abzeichen** zählen nur Aufbauen-Habits, Vermeiden hat eigene Abzeichen.
- **Joker:** Ein überbrückter Tag erhält die Serie, erhöht sie aber nicht und bringt keine
  XP. Wird der Tag später echt nachgetragen, kommt der Joker zurück. Bei Wochen-Habits
  prüft der automatische Einsatz am Sonntag, und nur wenn genau ein Check-in fehlt.
- **Joker-Grenze:** Die Grenze von 3 gilt für alle Quellen. Beim Level-Up mit 3 Jokern
  entfällt die Zeile „+1“ auf dem Feier-Screen, im Shop steht „max. 3“.
- **Level und Rückgängig:** Der erreichte Level wird gemerkt. Rückgängig machen senkt die
  XP, nie den Level, und Level-Boni, Abzeichen und Serien-Joker gibt es nur einmal.
  Abgehakt und wieder rückgängig gemacht lässt sich also nichts farmen.
- **Tagesquest:** „Alle n Habits“ zählt heute fällige Aufbauen-Habits plus Vermeiden-Habits
  ohne Rückfall. Wochen-Habits zählen, solange das Wochenziel offen ist. Wird das letzte
  Häkchen zurückgenommen, wird die Quest wieder abgezogen.
- **Perfekte Woche / Monat** brauchen mindestens zwei Habits, sonst wären sie mit einem
  einzigen Habit dasselbe wie die Serie.
- **Abzeichen mit „ד** zählen laut Text alle Check-ins des Bereichs. Angerufen und
  Nicht vergessen hängen daher am selben Zähler (Bereich Familie), nur Sport, Wasser
  und Lesen werden über das Tag am Icon unterschieden.
- **Jahreszeiten:** Eine Saison zählt ab 80 % bei mindestens 60 geplanten Check-ins, damit
  wenige Tage nicht reichen. Gemessen werden Habits mit festen Tagen.
- **Feiertage:** Bundesweite Feiertage DE sind eingebaut, weitere Tage und das Abschalten
  gehen unter Profil → Server.

- **Monatlich:** Die Vorlage „Blut spenden“ brauchte einen neuen Rhythmus. Die Serie zählt in
  Monaten, Bonus und Joker rechnen mit 30 Tagen je Monat. Joker gelten hier nicht.
- **Neue Abzeichen ohne Check-ins:** Digital, Geld und Umwelt enthalten fast nur
  Vermeiden-Vorlagen. Deshalb zählen „×“ dort saubere Tage statt Check-ins (Offline-Stunde,
  Mehrweg). Velo statt Auto zählt die Tags `auto` und `velo` zusammen.
- **Detox-Tag:** Ein ganzer Tag offline lässt sich nicht messen. Das Abzeichen gilt, wenn
  3 Digital-Vermeiden-Habits zugleich mindestens einen sauberen Tag in Serie haben.
- **`templates.json` korrigiert:** „Sportwetten und Glücksspiel“ trug das Tag `sport` und hätte
  Sport-Abzeichen ausgelöst, das Tag ist entfernt. „Essen bestellen“ hat das neue Tag
  `lieferdienst` für Selbstgemacht.

## Oberfläche

- **Jahr-Heatmap:** die ganzen Monate des gewählten Zeitraums, ohne Scrollen.
  Pro Tag gibt es bei einem einzelnen Habit nur erledigt oder nicht, die Farbstufen zeigen
  daher, wie weit der Tag in einer Serie liegt (1–2, 3–9, ab 10 Tagen).
- **Serien im Zeitverlauf:** Die Balken zeigen die Lage auf der Zeitachse des Habits, wie im
  Bild. „Davor“ ist die längste Serie vor dem Rekord.
- **Monatsstatistik:** Wie im Bild (28/29) zählt der laufende Tag nicht mit.
- **Neues Habit:** Zuerst nur Name, Vorschläge, „Beliebt“ und der Link zur Bibliothek, wie im
  Bild. Art, Häufigkeit und die übrigen Felder erscheinen, sobald eine Vorlage oder „… als
  eigenes Habit anlegen“ gewählt ist. Eine Vorlage übernimmt ihre Werte per Seitenaufruf
  (`/habits/new?template=…`), so geht es ohne JavaScript-Logik.
- **Vorschläge:** Beliebte zuerst, dann Namen mit passendem Anfang, dann der Rest.
- **Onboarding Schritt 2:** Pro Bereich bis zu 3 beliebte Vorlagen. Hat ein Bereich weniger
  als 2, wird aufgefüllt, ohne dasselbe Tag doppelt zu zeigen. Die Häufigkeit folgt
  `templates.json`, „30 Min. Sport“ ist dort täglich, im Bild 3× pro Woche.
- **Onboarding:** Startet, solange es keine Habits gibt und es nicht abgeschlossen wurde.
  „Überspringen“ steht unter dem Button und fehlt im Bild.
- **Profil:** Zusätzlich „Onboarding erneut starten“, das Bild zeigt nur die anderen Zeilen.
- **Abzeichen, Übersicht:** Pro Kategorie zwei Reihen wie im Bild, der Rest über „›“ oder
  die Filter. Die Kategorieansicht zeigt zusätzlich den Hinweistext. Geheime Abzeichen
  zeigen „???“ ohne Hinweis, auch wenn die Übersichtsgrafik ihn nennt.
- **Zusätzliche Bedienelemente:** Stift im Habit-Detail, Link „Neues Habit“ auf Heute, Tab
  „Verlauf“ bei Gönn dir, 18 statt 6 Icons, Wochentag-Auswahl bei „Eigene“. Die Kennzahlen
  und die Ziel-Belohnung stehen im Habit-Detail unter dem jeweiligen Bild-Inhalt.
- **Rückfall melden:** Tippen auf den Kreis bei „Kein Alkohol“, mit Rückfrage.
- **Nachtragen:** Ein Tag in Kalender oder Wochenleiste ändert nichts direkt, er öffnet ein Fenster
  für Erreicht oder Wert und Uhrzeit, und zwar bis zurück zum Start des Habits. Ein Klick daneben
  schließt es ohne Änderung, damit kein erreichter Tag aus Versehen verschwindet. XP, Taler und Serien folgen wie bei einem Klick an dem Tag, nachgetragen wird also bewusst
  auch belohnt. Der Wert ersetzt die Summe des Tages. Eingetragene Uhrzeiten zählen wie echte, auch
  für Muster und Zeit-Abzeichen, dürfen aber nicht in der Zukunft liegen. Vermeiden-Habits haben
  keinen Wert: Für sie schaltet ein Tag der letzten 7 Tage den Rückfall um, nach einer Rückfrage. Bei einem Wochenziel gilt der Wert für diesen einen Tag.

- **Mein Level:** Die Statistik rechnet aus dem Hauptbuch, „+X XP“ passt also zur Level-Karte.
  - Verglichen wird gleich lang: Diese Woche bis heute gegen dieselben Tage der Vorwoche.
  - Die Erfüllungsquote zählt offene Tage und laufende Wochen erst, wenn sie erreicht oder
    vorbei sind. Nur „Heute“ zählt das Offene mit, sonst wäre die Zahl morgens immer 100 %.
  - „Längste Serien“ zeigt die laufenden Serien, nicht die alten Rekorde, denn das Fenster
    „Deine Serien“ dahinter zeigt, was gerade läuft. Rekorde stehen je Habit dort.
  - „Heute“ zeigt im Diagramm die Woche, ein einzelner Balken sagt nichts.
- **Levelweg:** Das Bild zeigt nur einige Stufen. Gezeigt werden Level mit neuem Titel oder
  Abzeichen, dazu das aktuelle und das nächste. Titel und XP kommen aus der Level-Tabelle
  der Administration, die Beispielnamen aus dem Bild ersetzen sie nicht.
- **Verlauf:** Der Streifen zeigt die letzten 7 Tage statt der Kalenderwoche, sonst wäre am
  Montag nur ein Tag wählbar. Zwischenstände einer Quest werden nicht gespeichert, deshalb
  erscheint sie erst, wenn sie geschafft ist. Nachgetragenes steht ohne Uhrzeit unten.
- **Deine Serien:** Wochen-Habits gelten als in Gefahr, sobald die restlichen Tage gerade
  noch reichen, davor stehen sie bei „Sicher“. Ziele sind die Serien-Abzeichen, bei
  abgeschalteten Abzeichen deren Längen.

## Technik

- **Benutzer:** `HABITD_USER` gilt bei jedem Start, `HABITD_PASSWORD` nur beim ersten.
- **Zwei-Stufen-CSRF:** Jeder POST braucht das Session-Token, zusätzlich prüft Go
  `http.CrossOriginProtection` Origin und `Sec-Fetch-Site`, auch für den Login.
- **`Referrer-Policy: same-origin`** statt `no-referrer`: Bei `no-referrer` senden Browser
  `Origin: null`, und der Login scheitert über HTTP von anderen Geräten im Netz.
- **Alpine CSP-Build,** weil das normale Alpine `unsafe-eval` bräuchte. htmx läuft mit
  `allowEval: false`. Die CSP erlaubt nur `style`-Attribute inline für berechnete Breiten
  und Farben, keine `<style>`-Blöcke und keine Skripte.
- **Web-Push** ist mit der Standardbibliothek umgesetzt (VAPID, aes128gcm, getestet gegen den
  Testvektor aus RFC 8291). Die Nachricht läuft zwangsläufig über den Push-Dienst des Browsers.
- **Mehr Tabellen als vorgesehen:** `sessions`, `celebrations` und `push_subscriptions`. `areas`
  hat zusätzlich `key`, damit Abzeichen ein Umbenennen überleben, `habits` hat
  `weekly_target` (Check-ins pro Woche oder Monat).
- **Docker-Image:** `docker-compose.yml` nutzt `ghcr.io/seals187/habitd:latest`. Ein Workflow baut
  das Image bei jedem Tag `v*` und veröffentlicht es als `latest` und mit der Versionsnummer.
  `build: .` bleibt drin, damit `docker compose up --build` weiter aus dem Quellcode baut.
- **Version:** Das erste Release heißt `0.0.1`, die Bilder zeigen beispielhaft „v1.0“.
- **Changelog:** Jedes Release bekommt vor dem Tag einen Eintrag in `CHANGELOG.md`, der Text des
  GitHub-Release entspricht diesem Eintrag.
- **Seitenwechsel:** `hx-boost` auf `body` tauscht nur den Inhalt, damit nichts sichtbar neu lädt.
  Login, Setup, Abmelden und Passwort bleiben normale Formulare, weil sich dort die Session und
  mit ihr das CSRF-Token in `hx-headers` ändert. Export und Import brauchen ebenfalls den
  normalen Weg.
- **Zähler und Mengen:** Jeder Klick ist eine Zeile in `habit_entries`, „−“ löscht die letzte. Erst
  das Tagesziel erzeugt den normalen Check-in, deshalb bleiben Serie, XP, Quest und Abzeichen
  unverändert. Fällt die Summe unter das Ziel, wird der Check-in zurückgebucht. Teil-XP und ein
  abschaltbares 100-%-Ziel gibt es nicht. Klicks laufen durch eine eigene Warteschlange in
  `app.js`, weil jede Antwort die Knöpfe ersetzt und htmx ihre Warteschlange sonst verwirft.
- **Heute-Screen:** Karten nach `Karte-B-Chips.png`, darüber Level-Karte und Tagesquest. Der Ring
  ist bei Abhak- und Vermeiden-Habits der Knopf, weil der Entwurf dafür keinen eigenen zeigt: Abhaken
  beziehungsweise Rückfall melden. Das „−“ sitzt an der Ecke des „+“, sonst bleibt kein Platz für
  den Streifen. Teilweise gefüllte vergangene Tage zählen nicht als erreicht.
- **Habits löschen:** Der Verlauf verschwindet, das Ledger bleibt, verdiente XP und Taler also
  auch. Serien-Belohnungen des Habits werden archiviert. `AUTOINCREMENT` verhindert, dass ein
  neues Habit die ID und damit die Ledger-Referenzen eines gelöschten erbt.
- **Neu anfangen** ist bewusst getrennt von „Alle Habits löschen“: Nur hier verschwinden Ledger,
  Abzeichen und eingelöste Belohnungen. Das Installationsdatum rückt auf heute, sonst wären
  zeitbasierte Abzeichen wie „1 Jahr dabei“ sofort wieder erreicht.
- **Datentrennung:** Die Benutzer-ID steckt im Kontext, `db.Q` filtert jede Abfrage damit. Ohne
  Benutzer liefert eine Abfrage nichts statt alles. Schreibzugriffe auf fremde Zeilen enden über
  `execOne` als 404 statt als stiller Erfolg.
- **Bereiche** sind pro Benutzer: `areas` ohne `user_id` sind die globalen Startwerte, neue
  Benutzer bekommen eine Kopie. Sonst könnte ein Benutzer die Bereiche der anderen umbenennen.
- **Export und Import** vergeben neue IDs und schreiben Ledger-Referenzen um, weil ein Export von
  einem anderen Benutzer derselben Instanz stammen kann.
- **API-Token** gibt es weiterhin nur einen pro Instanz. Er handelt als erster aktiver Admin.
- **Startwerte der Levels** sind die bisherigen Werte, nicht die Beispielzahlen aus
  `Admin-1-Levels.png`: Basis 21,57, Exponent 1,822, 100 Taler und 1 Joker pro Level. Andere Werte
  hätten bestehenden Benutzern ihr Level oder ihre Belohnungen verändert.
- **Abzeichen-Regeln:** Serien-Abzeichen beziehen sich auf „Nur Aufbauen-Habits“, nicht wie im Bild
  auf „beliebiges Habit“, sonst zählten Vermeiden-Serien plötzlich mit. Der Bezug ersetzt auch
  den Schalter „Nur Aufbauen-Habits“ aus dem Bild. Regeln, die der Baukasten nicht ausdrücken
  kann (etwa Summen über mehrere Tags), bleiben `special` im Code. `hint` ist eine zusätzliche
  Spalte für die Kurzbeschrift unter der Medaille, sie entsteht bei eigenen Abzeichen aus dem Ziel.
- **Tempo** skaliert die Basis: gemütlich 0,7, normal 1, ehrgeizig 1,4.
- **Einladungen** liegen nur als Hash in der Datenbank, der Link ist einmal sichtbar.
- **Automatische Backups** entstehen vor jeder Migration einer bestehenden Datenbank.
- **Gönn dir:** „1× / Woche“ ist `weekly` zusätzlich zu `repeatable`, die Woche beginnt montags.
  Eine Einlösung lässt sich über den Toast zurücknehmen; die Rückbuchung trägt den Grund `redeem`,
  zählt also nicht als neu verdient. Einmal freigeschaltete Abzeichen bleiben dabei erhalten.
  Löschen entfernt auch den Verlauf der Belohnung, das Ledger bleibt. Ideen legen die Belohnung
  direkt an.
- **Tagesquests:** Pool, Plan und Ziehung sind global, gezogen wird pro Benutzer beim ersten Zugriff
  des Tages und nur einmal. Der Quest-Tag beginnt mit „Neue Quest ab“. Geplante Quests, die
  Wochenend-Regel und Events gehen vor, der Rest kommt aus dem Pool. „Vermeiden“ entscheidet sich
  erst, wenn der Tag vorbei ist. Die erste Quest eines Tages nutzt weiter die Ledger-Referenz
  `quest:<datum>`, damit eine alte Auszahlung vom selben Tag nicht doppelt bezahlt wird.
- **Mengen** sind Dezimalzahlen, Summen werden auf drei Stellen gerundet, damit 30 × 0,1 das Ziel 3
  erreicht. Die Spalten mussten dafür nicht geändert werden, SQLite speichert sie als REAL.
- **Alles zurücksetzen** löscht inzwischen auch die Belohnungen. Sie stammten bei frischen
  Installationen nur aus den Demo-Daten.
- **Rückfragen** laufen über einen eigenen Dialog statt `window.confirm`. Der erste Satz von
  `hx-confirm` wird zum Titel, `data-yes` beschriftet den Knopf, Standard ist „Löschen“.
- **Abgelehnte Formulare** antworten mit der ganzen Seite. Fehlerantworten mit `text/html` werden
  deshalb eingesetzt, reiner Text bleibt ein kurzer Hinweis.
- **Habits mit Einheit** zeigen in der Statistik die Menge statt der Serie. Alles wird beim Aufruf
  aus den Einträgen berechnet, gespeichert wird nichts zusätzlich. Das Zeitraum-Ziel ist die Zahl
  der geplanten Tage mal Tagesziel und beginnt frühestens am Start des Habits. Ein laufender Monat
  wird mit denselben Tagen des Vormonats verglichen, ein laufender Jahreszeitraum mit demselben
  Stück des Zeitraums davor.
- **Jahresansicht:** Sie zeigt 6 oder 12 ganze Monate bis zum laufenden Monat statt des
  Kalenderjahrs, Vorgabe sind 6. Monate vor dem Start bleiben als Stummel stehen, damit ein neues
  Habit nicht nur wenige Balken hat. Die Länge liegt pro Gerät im Cookie `year_span`.
- **Icons in der Seite:** Der Icon-Satz steckt in jeder Seite statt in einer eigenen Datei. Das kostet
  rund 30 KB pro Seite, dafür blinken die Icons beim Neuladen nicht mehr.
- **Tage vor dem Anlegen:** Ein nachgetragener Tag vor dem Anlegen zählt in der Statistik des Habits
  mit, „seit“ rückt auf diesen Tag. Quests und Abzeichen rechnen weiter ab dem Anlegen.
- **Ziehen zum Aktualisieren:** Die Geste des Browsers bleibt aus, weil ihr Überziehen die Menüleiste
  anhebt und auf Seiten mit Menüleiste das Fenster gar nicht mehr scrollt. Die App bringt eine eigene
  Geste mit: Ganz oben nach unten ziehen lädt die Seite neu, im Browser wie in der installierten App.
- **Zeiträume wechseln:** Wischen mit dem Finger oder mit gehaltener Maus nutzt die Pfeile im Kopf
  der Karte. Nach rechts geht es zurück, nach links vor, aber nicht über den laufenden Zeitraum
  hinaus. Die Adresse wird ersetzt, damit „Zurück“ das Habit verlässt und nicht jede Woche einzeln
  durchläuft.
- **Heute frei:** Habits mit festen Wochentagen bleiben an freien Tagen unten auf „Heute“ sichtbar.
  Ein erreichtes Wochen- oder Monatsziel klappt die Karte bis zum Ende des Zeitraums ein.
- **Erreichte Karten** lassen sich aufklappen. Das gilt nur für den laufenden Tag, am nächsten Tag
  klappt eine weiterhin erreichte Karte wieder ein, damit „Heute“ übersichtlich bleibt.
- **Ein- und Aufklappen** liegt pro Gerät in Cookies statt in der Datenbank. Der Klick ändert nur
  die Karte im Browser und braucht keinen Server, gerendert wird wieder aus den Cookies. Das gilt
  auch für die Level-Anzeige, die eingeklappt mit einer Zeile auskommt.
- **Erreichte Habits** teilen sich eine Zeile mit Bereichen, Fortschritt des Tages, besten Serien und
  dem, was die Check-ins von heute gebracht haben. Die Serien stehen getrennt nach Tagen und Wochen,
  weil sich beides nicht vergleichen lässt. Ein Klick und „Alle aufklappen“ öffnen die kompakte
  Liste, Details öffnet jede Karte einzeln. Eine offene Liste gilt wie eine offene Karte nur für
  diesen Tag und dieses Gerät.
- **Muster & Tipps** rechnet `internal/tips` nur aus den eigenen Daten, ohne KI und ohne dass etwas
  den Server verlässt. Jede Regel ist eine kleine Funktion mit Test. Regeln über den Tag brauchen
  7 Tage Laufzeit, Regeln über die Uhrzeit 5 echte Zeitstempel. Nachgetragene Tage liegen auf 12:00
  und zählen dafür nicht. Rückfälle haben keine Uhrzeit, Vermeiden kennt darum keinen Stundenstreifen.
- **Größe der Sparziele** gilt für alle Sparziele zusammen und pro Gerät, wie bei den Habits.
- **Heute aktualisiert sich selbst:** Alle 15 Sekunden fragt die sichtbare Seite ihren Stand ab und
  tauscht ihn nur bei einer Änderung, beim Zurückkehren in den Tab sofort. Polling statt Server-Sent
  Events, weil es ohne offene Verbindungen und ohne Änderungen am Dienst auskommt. Offene Dialoge,
  Menüs und laufende Klicks haben Vorrang, die Antwort wird dann verworfen.
- **Quest-Habits:** Der Quest-Pool gehört der Instanz und wird im Admin-Bereich angepasst, Habits
  gehören den Nutzern. Darum wählt jeder selbst, welche Habits für die „Alle Habits“-Quests zählen.
  Die Wahl gilt ab sofort auch für die Quest des Tages, vergangene Tage bleiben, wie sie sind.
- **Habit-Größe:** liegt pro Gerät in einem Cookie statt im Konto, weil ein Handy andere Größen
  braucht als ein Desktop. Der Server liest das Cookie, so springt die Seite beim Laden nicht.
- **Figuren:** Die 16 Figuren mit je 8 Stufen sind nachgezeichnete SVG aus `tools/genfigures`, weil
  die Vorlage nur ein PNG ist. Die Stufe folgt dem mittleren Füllstand der Ziele von heute, jeder
  Klick zählt also. Vermeiden und „Heute frei“ zählen nicht mit, gewählt wird pro Nutzer.
- **Gruppen auf „Heute“:** Offen und Erreicht ersetzen die Überschrift „Aufbauen“. Vermeiden bleibt
  eine eigene Gruppe, weil diese Habits erst nach Tagesende als geschafft gelten.
- **Mehr als das Ziel** zeigt die Karte in Gold: eine zweite Runde im Kreis, weitere Balken und der
  Wert neben der Woche. Das ist nur Anzeige aus den Einträgen, XP und Taler gibt es weiter einmal.
- **Vorlagen** öffnen immer zuerst das Formular. Nur das Onboarding legt mehrere direkt an.
- **Wochenziel:** Zähler und Mengen können pro Woche gelten. Das Habit läuft dann als „1× pro
  Woche“, das Ziel sammelt sich über die Einträge der Woche, und der Check-in entsteht an dem Tag,
  an dem es erreicht wird. So bleiben Serie, XP und Taler unverändert, sie fallen einmal pro Woche
  an. Ein Tipp im Kalender bucht dort einen Klick oder leert den Tag, er füllt nie das ganze Ziel.
- **Komprimierung:** Nur CSS, JS und das Sprite werden mit gzip ausgeliefert. Seiten bleiben
  unkomprimiert, weil das CSRF-Token dort neben reflektierten Eingaben steht (BREACH).
- **Migrationen** laufen mit deaktivierten Foreign Keys auf einer eigenen Verbindung, weil
  SQLite für eine neue CHECK-Regel die Tabelle neu aufbauen muss. Danach prüft
  `foreign_key_check` die Daten.
- **`internal/service`** ergänzt die vorgeschlagene Struktur: Dort laufen die Transaktionen,
  `internal/game` bleibt pur und ohne Datenbank.
- **Demo-Daten:** Die Salden stimmen exakt mit den Bildern überein. Der Seed zeigt mehr
  freigeschaltete Abzeichen (24 statt 18), weil die Regeln auf den Bild-Daten mehr
  auslösen, etwa Jongleur bei 5 Habits. Die Daten sind nicht geschönt, sonst käme beim ersten
  Abhaken eine Serie von Feiern. Lesen existiert 246 Tage, damit 212 Tage 86 % ergeben, obwohl
  das Profil „dabei seit 212 Tagen“ zeigt: So widersprechen sich die Bilder selbst.
