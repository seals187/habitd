package server

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/seals187/habitd/internal/model"
)

func (c *client) coins(amount int) {
	c.t.Helper()
	if err := c.q().AddLedger(c.svc.Now(), model.LedgerCoins, amount, "test", ""); err != nil {
		c.t.Fatal(err)
	}
}

func (c *client) reward(rw model.Reward) int64 {
	c.t.Helper()
	if rw.Mode == "" {
		rw.Mode = model.RewardCoins
	}
	if err := c.q().SaveReward(rw); err != nil {
		c.t.Fatal(err)
	}
	all, err := c.q().Rewards()
	if err != nil {
		c.t.Fatal(err)
	}
	return all[len(all)-1].ID
}

// quietShop marks the first-redemption badges as held: they would answer the
// next redemption with a celebration instead of the page.
func (c *client) quietShop() {
	c.t.Helper()
	for _, key := range []string{"erste-belohnung", "erster-taler"} {
		if err := c.q().UnlockBadge(key, c.svc.Now()); err != nil {
			c.t.Fatal(err)
		}
	}
}

func (c *client) balance() model.Balances {
	c.t.Helper()
	bal, err := c.q().Balances(c.svc.Today())
	if err != nil {
		c.t.Fatal(err)
	}
	return bal
}

func TestEmptyRewardsOfferIdeas(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	_, html := c.get("/rewards")
	for _, want := range []string{"Noch keine Belohnungen", "Erste Belohnung anlegen", "Lieblingskaffee", "Ausflug"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q", want)
		}
	}
	resp, html := c.post("/reward-ideas/kaffee", true)
	if resp.StatusCode != http.StatusOK || !strings.Contains(html, "Lieblingskaffee") || strings.Contains(html, "Noch keine Belohnungen") {
		t.Fatalf("idea: %s", resp.Status)
	}
	all, _ := c.q().Rewards()
	if len(all) != 1 || all[0].Icon != "cup" || all[0].Price != 80 {
		t.Fatalf("rewards: %+v", all)
	}
	if resp, _ := c.post("/reward-ideas/gibtsnicht", true); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown idea: %s", resp.Status)
	}
}

func TestRewardSections(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	habit := c.habit("Lesen", "build")
	c.coins(206)
	c.reward(model.Reward{Title: "Billig", Price: 50, Repeatable: true})
	c.reward(model.Reward{Title: "Teuer", Price: 250})
	c.reward(model.Reward{Title: "Buch", Mode: model.RewardStreak, HabitID: habit, StreakTarget: 30})
	paused := c.reward(model.Reward{Title: "Ruht", Price: 10})
	c.post(fmt.Sprintf("/rewards/%d/pause?paused=on", paused), true)
	_, html := c.get("/rewards")
	for _, want := range []string{"Kannst du dir leisten", "Sparziele", "Für Serien", "Pausiert", "mehrfach", "206/250", "noch 44", "0/30 Tage", "Serie · Lesen"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if resp, _ := c.post(fmt.Sprintf("/rewards/%d/redeem", paused), true); resp.StatusCode != http.StatusConflict {
		t.Fatalf("paused reward redeemed: %s", resp.Status)
	}
	c.post(fmt.Sprintf("/rewards/%d/pause", paused), true)
	if rw, _ := c.q().Reward(paused); rw.Paused {
		t.Fatal("still paused")
	}
}

func TestRedeemCanBeUndone(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	c.quietShop()
	c.coins(100)
	earned := c.balance().CoinsEarned
	id := c.reward(model.Reward{Title: "Kaffee", Price: 80, Repeatable: true})
	_, html := c.post(fmt.Sprintf("/rewards/%d/redeem", id), true)
	if !strings.Contains(html, "„Kaffee“ eingelöst · −80 Taler") || !strings.Contains(html, "hx-swap-oob") {
		t.Fatalf("toast: %s", html)
	}
	if got := c.balance().Coins; got != 20 {
		t.Fatalf("coins: %d", got)
	}
	m := regexp.MustCompile(`/rewards/redemptions/(\d+)/undo`).FindStringSubmatch(html)
	if m == nil {
		t.Fatal("no undo")
	}
	if resp, _ := c.post(m[0], true); resp.StatusCode != http.StatusOK {
		t.Fatalf("undo: %s", resp.Status)
	}
	bal := c.balance()
	if bal.Coins != 100 || bal.CoinsEarned != earned {
		t.Fatalf("refund: %+v", bal)
	}
	if red, _ := c.q().Redemptions(); len(red) != 0 {
		t.Fatalf("redemptions: %+v", red)
	}
	if resp, _ := c.post(m[0], true); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("second undo: %s", resp.Status)
	}
}

func TestFreeRedeemCostsNothing(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	c.quietShop()
	id := c.reward(model.Reward{Title: "Kino", Price: 500})
	if resp, _ := c.post(fmt.Sprintf("/rewards/%d/redeem", id), true); resp.StatusCode != http.StatusConflict {
		t.Fatalf("without Taler: %s", resp.Status)
	}
	if resp, _ := c.post(fmt.Sprintf("/rewards/%d/free", id), true); resp.StatusCode != http.StatusOK {
		t.Fatalf("free: %s", resp.Status)
	}
	if got := c.balance().Coins; got != 0 {
		t.Fatalf("coins: %d", got)
	}
	red, _ := c.q().Redemptions()
	if len(red) != 1 || red[0].PricePaid != 0 {
		t.Fatalf("redemption: %+v", red)
	}
	if _, html := c.get("/rewards?tab=history"); !strings.Contains(html, "ohne Taler") {
		t.Fatal("history")
	}
}

func TestWeeklyRewardWaitsForNextWeek(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	c.quietShop()
	c.coins(100)
	id := c.reward(model.Reward{Title: "Ausschlafen", Price: 30, Repeatable: true, Weekly: true})
	if resp, _ := c.post(fmt.Sprintf("/rewards/%d/redeem", id), true); resp.StatusCode != http.StatusOK {
		t.Fatalf("first: %s", resp.Status)
	}
	if resp, _ := c.post(fmt.Sprintf("/rewards/%d/redeem", id), true); resp.StatusCode != http.StatusConflict {
		t.Fatalf("second: %s", resp.Status)
	}
	if _, html := c.get("/rewards"); !strings.Contains(html, "nächste Woche wieder") {
		t.Fatal("no hint")
	}
}

func TestRewardDuplicateDeleteAndForm(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	c.quietShop()
	id := c.reward(model.Reward{Title: "Kino", Price: 40, Icon: "eye", Weekly: true, Repeatable: true})
	c.post(fmt.Sprintf("/rewards/%d/duplicate", id), true)
	all, _ := c.q().Rewards()
	if len(all) != 2 || all[1].Title != "Kino (Kopie)" || all[1].Icon != "eye" || !all[1].Weekly {
		t.Fatalf("copy: %+v", all)
	}
	form := url.Values{"csrf": {c.csrf}, "title": {"Neu"}, "mode": {"coins"}, "price": {"70"}, "icon": {"moon"}, "rhythm": {"always"}, "show_today": {"on"}}
	if resp, _ := c.do("POST", fmt.Sprintf("/rewards/%d/edit", id), form, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save: %s", resp.Status)
	}
	rw, _ := c.q().Reward(id)
	if rw.Title != "Neu" || rw.Icon != "moon" || rw.Weekly || !rw.Repeatable || !rw.ShowToday {
		t.Fatalf("reward: %+v", rw)
	}
	form.Set("icon", "<script>")
	form.Set("rhythm", "once")
	c.do("POST", fmt.Sprintf("/rewards/%d/edit", id), form, nil)
	if rw, _ = c.q().Reward(id); rw.Icon != "moon" || rw.Repeatable {
		t.Fatalf("reward: %+v", rw)
	}
	c.coins(100)
	form.Set("rhythm", "always")
	c.do("POST", fmt.Sprintf("/rewards/%d/edit", id), form, nil)
	c.post(fmt.Sprintf("/rewards/%d/redeem", id), true)
	if _, html := c.get(fmt.Sprintf("/rewards/%d/edit", id)); !strings.Contains(html, "1× eingelöst · zuletzt") || !strings.Contains(html, "Belohnung löschen") {
		t.Fatal("edit form")
	}
	if resp, _ := c.post(fmt.Sprintf("/rewards/%d/delete", id), true); resp.StatusCode != http.StatusOK {
		t.Fatalf("delete: %s", resp.Status)
	}
	if resp, _ := c.get(fmt.Sprintf("/rewards/%d/edit", id)); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("still there: %s", resp.Status)
	}
}

func TestTodayShowsLevelQuestAndTreats(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	c.quietShop()
	habit := c.habit("Lesen", "build")
	c.habit("Sport", "build")
	c.coins(100)
	cheap := c.reward(model.Reward{Title: "Kleiner Luxus", Price: 50, ShowToday: true, Repeatable: true})
	c.reward(model.Reward{Title: "Zu teuer", Price: 5000, ShowToday: true})
	c.reward(model.Reward{Title: "Nicht gewollt", Price: 10})
	_, html := c.get("/")
	if !strings.Contains(html, "Kleiner Luxus") || strings.Contains(html, "Zu teuer") || strings.Contains(html, "Nicht gewollt") {
		t.Fatal("rewards on Today")
	}
	for _, want := range []string{"level-tile", "bis Level 2", "&#43;0 XP", "Alle Habits des Tages abhaken", "&#43;50 Taler"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q", want)
		}
	}
	c.post(fmt.Sprintf("/habits/%d/check", habit), false)
	_, html = c.get("/")
	if strings.Contains(html, "&#43;0 XP") {
		t.Fatal("xp of today")
	}
	if strings.Count(html, `<i class="on"></i>`) < 1 {
		t.Fatal("quest dots")
	}
	for {
		id, _, _, err := c.q().NextCelebration()
		if err != nil {
			break
		}
		if err := c.q().DeleteCelebration(id); err != nil {
			t.Fatal(err)
		}
	}
	resp, body := c.post(fmt.Sprintf("/rewards/%d/redeem?from=today", cheap), true)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "today-head") || !strings.Contains(body, "from=today") {
		t.Fatalf("redeem from Today: %s", resp.Status)
	}
}

// Both modes share one form, the hidden fields must never fail validation.
func TestRewardFormHasNoInvalidHiddenFields(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	habit := c.habit("Lesen", "build")
	coins := c.reward(model.Reward{Title: "Taler", Price: 50})
	streak := c.reward(model.Reward{Title: "Serie", Mode: model.RewardStreak, HabitID: habit, StreakTarget: 5})
	for _, id := range []int64{coins, streak} {
		_, html := c.get(fmt.Sprintf("/rewards/%d/edit", id))
		if strings.Contains(html, `value="0"`) {
			t.Fatalf("reward %d has a zero field", id)
		}
	}
}
