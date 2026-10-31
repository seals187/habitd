package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"time"
)

// envVars lists the settings of the instance; secrets only show that they are set.
var envVars = []string{"TZ", "HABITD_PORT", "HABITD_DB", "HABITD_RUNTIME", "HABITD_ADMIN_USER", "HABITD_USER", "HABITD_ADMIN_PASSWORD", "HABITD_PASSWORD",
	"HABITD_ALLOW_SIGNUP", "HABITD_INSECURE_COOKIES", "HABITD_COOKIE_SECURE", "HABITD_API_TOKEN"}

var secretVars = []string{"HABITD_ADMIN_PASSWORD", "HABITD_PASSWORD", "HABITD_API_TOKEN"}

type envRow struct {
	Name  string
	Value string
	Set   bool
}

func envRows() []envRow {
	rows := make([]envRow, 0, len(envVars))
	for _, name := range envVars {
		value, set := os.LookupEnv(name)
		set = set && value != ""
		if set && slices.Contains(secretVars, name) {
			value = "gesetzt"
		}
		rows = append(rows, envRow{Name: name, Value: value, Set: set})
	}
	return rows
}

func sizeLabel(bytes int64) string {
	if bytes < 1<<20 {
		return fmt.Sprintf("%d KB", bytes>>10)
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
}

func uptimeLabel(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%d Min.", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d Std.", int(d.Hours()))
	}
	return fmt.Sprintf("%d Tage", int(d.Hours()/24))
}

func (s *Server) adminSystem(w http.ResponseWriter, r *http.Request) error {
	size, err := s.svc.Store.Size()
	if err != nil {
		return err
	}
	users, err := s.svc.Store.Q(r.Context()).Users()
	if err != nil {
		return err
	}
	return s.view.Render(w, http.StatusOK, "admin_system", s.adminPage(r, "System", "system", map[string]any{
		"Version": s.cfg.Version, "Size": sizeLabel(size), "Users": len(users), "Uptime": uptimeLabel(time.Since(s.started)), "Env": envRows(),
	}))
}

// adminBackup streams a consistent copy. It lands next to the database, because
// the image has no temp directory.
func (s *Server) adminBackup(w http.ResponseWriter, r *http.Request) error {
	path := s.svc.Store.Path() + ".download-" + randomToken()[:12]
	defer os.Remove(path)
	if err := s.svc.Store.BackupTo(r.Context(), path); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="habitd-%s.db"`, s.svc.Now().Format("2006-01-02")))
	_, err = io.Copy(w, file)
	return err
}
