package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/service"
)

const maxImportSize = 32 << 20

type profileData struct {
	Progress     game.Progress
	Title        string
	DaysIn       int
	XP           int
	CoinsEarned  int
	Longest      int
	Redeemed     int
	Habits       int
	Areas        int
	ReminderTime string
	MaxJokers    int
	Version      string
	Runtime      string
}

func (s *Server) profile(w http.ResponseWriter, r *http.Request) error {
	q := s.svc.Store.Q(r.Context())
	world, err := s.svc.World(q)
	if err != nil {
		return err
	}
	areas, err := q.Areas()
	if err != nil {
		return err
	}
	on, err := q.Setting(service.KeyReminderOn, "0")
	if err != nil {
		return err
	}
	reminder, err := q.Setting(service.KeyReminderTime, "20:00")
	if err != nil {
		return err
	}
	if on != "1" {
		reminder = "aus"
	}
	progress := game.ProgressFor(world.Balances.XP, world.Level)
	d := profileData{Progress: progress, Title: game.Title(progress.Level), DaysIn: int(world.Today - world.Installed),
		XP: world.Balances.XP, CoinsEarned: world.Balances.CoinsEarned, Redeemed: len(world.Redemptions),
		Areas: len(areas), ReminderTime: reminder, MaxJokers: game.MaxJokers, Version: s.cfg.Version, Runtime: s.cfg.Runtime}
	for _, h := range world.Habits {
		st := h.StreakAt(world.Today)
		unit := 1
		if st.Weekly {
			unit = 7
		}
		d.Longest = max(d.Longest, st.Longest*unit)
		if !h.Archived {
			d.Habits++
		}
	}
	return s.view.Render(w, http.StatusOK, "profile", s.page(r, "Profil", "profile", d))
}

func (s *Server) areas(w http.ResponseWriter, r *http.Request) error {
	areas, err := s.svc.Store.Q(r.Context()).Areas()
	if err != nil {
		return err
	}
	return s.view.Render(w, http.StatusOK, "areas", s.page(r, "Bereiche verwalten", "profile", map[string]any{"Areas": areas}))
}

func (s *Server) areaSave(w http.ResponseWriter, r *http.Request) error {
	a := model.Area{Name: strings.TrimSpace(r.FormValue("name")), Color: r.FormValue("color"), Icon: r.FormValue("icon")}
	a.ID, _ = strconv.ParseInt(r.FormValue("id"), 10, 64)
	if a.Name == "" || !colorPattern.MatchString(a.Color) {
		return badRequest("Name und Farbe sind nötig")
	}
	if a.Icon == "" {
		a.Icon = "heart"
	}
	if err := s.svc.Store.Q(r.Context()).SaveArea(a); err != nil {
		return err
	}
	return redirect(w, r, "/settings/areas")
}

func (s *Server) areaDelete(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.svc.Store.Q(r.Context()).DeleteArea(id); err != nil {
		return httpError{http.StatusConflict, "Bereich enthält noch Habits"}
	}
	return redirect(w, r, "/settings/areas")
}

var timePattern = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

func (s *Server) reminders(w http.ResponseWriter, r *http.Request) error {
	q := s.svc.Store.Q(r.Context())
	on, err := q.Setting(service.KeyReminderOn, "0")
	if err != nil {
		return err
	}
	at, err := q.Setting(service.KeyReminderTime, "20:00")
	if err != nil {
		return err
	}
	keys, err := s.svc.PushKeys(r.Context())
	if err != nil {
		return err
	}
	subs, err := q.PushSubscriptions()
	if err != nil {
		return err
	}
	return s.view.Render(w, http.StatusOK, "reminders", s.page(r, "Erinnerungen", "profile", map[string]any{
		"On": on == "1", "Time": at, "Public": keys.Public, "Devices": len(subs),
	}))
}

func (s *Server) remindersSave(w http.ResponseWriter, r *http.Request) error {
	at := r.FormValue("time")
	if !timePattern.MatchString(at) {
		return badRequest("Ungültige Uhrzeit")
	}
	on := "0"
	if r.FormValue("enabled") == "on" {
		on = "1"
	}
	err := s.svc.Store.Tx(r.Context(), func(q *db.Q) error {
		if err := q.SetSetting(service.KeyReminderOn, on); err != nil {
			return err
		}
		return q.SetSetting(service.KeyReminderTime, at)
	})
	if err != nil {
		return err
	}
	return redirect(w, r, "/settings/reminders")
}

type pushRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

func (s *Server) pushSubscribe(w http.ResponseWriter, r *http.Request) error {
	var body pushRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&body); err != nil {
		return badRequest("Ungültige Anfrage")
	}
	if !strings.HasPrefix(body.Endpoint, "https://") || body.Keys.P256dh == "" || body.Keys.Auth == "" {
		return badRequest("Ungültige Anfrage")
	}
	err := s.svc.Store.Q(r.Context()).SavePushSubscription(db.PushSubscription{Endpoint: body.Endpoint, P256dh: body.Keys.P256dh, Auth: body.Keys.Auth})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) pushUnsubscribe(w http.ResponseWriter, r *http.Request) error {
	var body pushRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&body); err != nil {
		return badRequest("Ungültige Anfrage")
	}
	if err := s.svc.Store.Q(r.Context()).DeletePushSubscription(body.Endpoint); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) jokerRules(w http.ResponseWriter, r *http.Request) error {
	q := s.svc.Store.Q(r.Context())
	mode, err := q.Setting(service.KeyJokerMode, service.JokerAuto)
	if err != nil {
		return err
	}
	bal, err := q.Balances(s.svc.Today())
	if err != nil {
		return err
	}
	return s.view.Render(w, http.StatusOK, "joker", s.page(r, "Joker-Regeln", "profile", map[string]any{
		"Auto": mode == service.JokerAuto, "Jokers": bal.Jokers, "Max": game.MaxJokers, "Price": game.JokerPrice,
	}))
}

func (s *Server) jokerRulesSave(w http.ResponseWriter, r *http.Request) error {
	mode := service.JokerManual
	if r.FormValue("mode") == service.JokerAuto {
		mode = service.JokerAuto
	}
	if err := s.svc.Store.Q(r.Context()).SetSetting(service.KeyJokerMode, mode); err != nil {
		return err
	}
	return redirect(w, r, "/settings/joker")
}

func (s *Server) backup(w http.ResponseWriter, r *http.Request) error {
	return s.view.Render(w, http.StatusOK, "backup", s.page(r, "Export & Backup", "profile", map[string]any{"Error": r.FormValue("error")}))
}

func (s *Server) exportData(w http.ResponseWriter, r *http.Request) error {
	e, err := s.svc.Store.Q(r.Context()).DumpAll()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="habitd-%s.json"`, s.svc.Today()))
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(e)
}

func (s *Server) importData(w http.ResponseWriter, r *http.Request) error {
	file, _, err := r.FormFile("file")
	if err != nil {
		return badRequest("Bitte wähle eine Datei")
	}
	defer file.Close()
	var e db.Export
	if err := json.NewDecoder(io.LimitReader(file, maxImportSize)).Decode(&e); err != nil {
		return redirect(w, r, "/settings/backup?error=Datei+nicht+lesbar")
	}
	if err := s.svc.Import(r.Context(), &e); err != nil {
		var he httpError
		if errors.As(err, &he) {
			return err
		}
		return redirect(w, r, "/settings/backup?error=Import+fehlgeschlagen")
	}
	return redirect(w, r, "/")
}

func (s *Server) serverInfo(w http.ResponseWriter, r *http.Request) error {
	q := s.svc.Store.Q(r.Context())
	on, err := q.Setting(service.KeyHolidaysOn, "1")
	if err != nil {
		return err
	}
	extra, err := q.Setting(service.KeyHolidayExtra, "")
	if err != nil {
		return err
	}
	return s.view.Render(w, http.StatusOK, "server", s.page(r, "Server", "profile", map[string]any{
		"Version": s.cfg.Version, "Runtime": s.cfg.Runtime, "Go": runtime.Version(), "Zone": s.svc.Loc.String(),
		"Holidays": on == "1", "Extra": extra, "API": s.cfg.APIToken != "",
	}))
}

var holidayPattern = regexp.MustCompile(`^(\d{2}-\d{2})?([ ,]+\d{2}-\d{2})*$`)

func (s *Server) holidaysSave(w http.ResponseWriter, r *http.Request) error {
	extra := strings.TrimSpace(r.FormValue("extra"))
	if !holidayPattern.MatchString(extra) {
		return badRequest("Format: MM-TT, getrennt durch Komma")
	}
	on := "0"
	if r.FormValue("enabled") == "on" {
		on = "1"
	}
	err := s.svc.Store.Tx(r.Context(), func(q *db.Q) error {
		if err := q.SetSetting(service.KeyHolidaysOn, on); err != nil {
			return err
		}
		return q.SetSetting(service.KeyHolidayExtra, extra)
	})
	if err != nil {
		return err
	}
	return redirect(w, r, "/settings/server")
}

func (s *Server) serviceWorker(w http.ResponseWriter, r *http.Request) error {
	body, err := fsRead(s.static, "sw.js")
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, err = w.Write([]byte(strings.ReplaceAll(string(body), "__VERSION__", s.view.Version)))
	return err
}

func (s *Server) manifest(w http.ResponseWriter, r *http.Request) error {
	body, err := fsRead(s.static, "manifest.webmanifest")
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	_, err = w.Write(body)
	return err
}

func (s *Server) offline(w http.ResponseWriter, r *http.Request) error {
	return s.view.Render(w, http.StatusOK, "offline", s.page(r, "Offline", "", nil))
}
