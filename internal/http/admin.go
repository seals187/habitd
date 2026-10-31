package server

import (
	"net/http"

	"github.com/seals187/habitd/internal/view"
)

type handlerFunc = func(http.ResponseWriter, *http.Request) error

// adminOnly guards the admin area. The user behind it comes from the database
// on every request, so a lost role counts at once. Others get a plain 404: they
// do not need to know the area exists.
func adminOnly(h handlerFunc) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		if !currentUser(r).IsAdmin() {
			return notFound()
		}
		return h(w, r)
	}
}

func (s *Server) adminPage(r *http.Request, title, nav string, data any) view.Page {
	p := s.page(r, title, "", data)
	p.Nav = nav
	return p
}

func (s *Server) adminHome(w http.ResponseWriter, r *http.Request) error {
	users, err := s.svc.Store.Q(r.Context()).Users()
	if err != nil {
		return err
	}
	rules := s.svc.Rules()
	return s.view.Render(w, http.StatusOK, "admin_home", s.adminPage(r, "Übersicht", "home", map[string]int{
		"Users": len(users), "Levels": len(rules.Levels), "Badges": rules.ActiveBadges(),
	}))
}

func (s *Server) adminPlaceholder(title, nav, text string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		return s.view.Render(w, http.StatusOK, "admin_placeholder", s.adminPage(r, title, nav, text))
	}
}
