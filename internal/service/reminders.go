package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/push"
)

// PushKeys returns the VAPID keys, creating them on first use.
func (s *Service) PushKeys(ctx context.Context) (push.Keys, error) {
	var keys push.Keys
	err := s.Store.Tx(ctx, func(q *db.Q) error {
		var err error
		if keys.Public, err = q.Setting(KeyVAPIDPublic, ""); err != nil {
			return err
		}
		if keys.Private, err = q.Setting(KeyVAPIDPrivate, ""); err != nil {
			return err
		}
		if keys.Public != "" && keys.Private != "" {
			return nil
		}
		if keys, err = push.GenerateKeys(); err != nil {
			return err
		}
		if err := q.SetSetting(KeyVAPIDPublic, keys.Public); err != nil {
			return err
		}
		return q.SetSetting(KeyVAPIDPrivate, keys.Private)
	})
	return keys, err
}

// RunBackground settles finished days and sends reminders until ctx ends.
func (s *Service) RunBackground(ctx context.Context) {
	client := &http.Client{Timeout: 15 * time.Second}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Settle(ctx); err != nil {
				slog.Error("settle failed", "err", err)
			}
			if err := s.remind(ctx, client); err != nil {
				slog.Error("reminder failed", "err", err)
			}
		}
	}
}

func (s *Service) remind(ctx context.Context, client *http.Client) error {
	q := s.Store.Q(ctx)
	on, err := q.Setting(KeyReminderOn, "0")
	if err != nil || on != "1" {
		return err
	}
	at, err := q.Setting(KeyReminderTime, "20:00")
	if err != nil {
		return err
	}
	today := s.Today().String()
	if sent, err := q.Setting(KeyReminderSent, ""); err != nil || sent == today {
		return err
	}
	if s.Now().Format("15:04") < at {
		return nil
	}
	if err := q.SetSetting(KeyReminderSent, today); err != nil {
		return err
	}
	w, err := s.World(q)
	if err != nil {
		return err
	}
	quest := game.QuestFor(w.Habits, w.Today)
	if quest.Complete || quest.Total == 0 {
		return nil
	}
	return s.sendAll(ctx, client, map[string]string{
		"title": "habitd",
		"body":  "Noch offen heute: " + strconv.Itoa(quest.Total-quest.Done) + " Habits. Deine Serie wartet!",
	})
}

func (s *Service) sendAll(ctx context.Context, client *http.Client, message map[string]string) error {
	q := s.Store.Q(ctx)
	subs, err := q.PushSubscriptions()
	if err != nil {
		return err
	}
	keys, err := s.PushKeys(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(message)
	if err != nil {
		return err
	}
	for _, sub := range subs {
		err := push.Send(ctx, client, push.Subscription{Endpoint: sub.Endpoint, P256dh: sub.P256dh, Auth: sub.Auth}, keys, body)
		switch {
		case errors.Is(err, push.ErrGone):
			if err := q.DeletePushSubscription(sub.Endpoint); err != nil {
				return err
			}
		case err != nil:
			slog.Warn("push failed", "err", err)
		}
	}
	return nil
}
