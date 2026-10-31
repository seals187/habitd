package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/service"
)

const adminUsage = `usage:
  habitd admin list
  habitd admin create <user> [--admin]
  habitd admin reset-password <user>
  habitd admin promote <user>
  habitd admin demote <user>`

// adminCommand is the emergency access: it works directly on the database, also
// while the server runs, because SQLite WAL allows short parallel transactions.
func adminCommand(ctx context.Context, svc *service.Service, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(adminUsage)
	}
	cmd, args := args[0], args[1:]
	if cmd == "list" {
		return adminList(ctx, svc, out)
	}
	if len(args) == 0 {
		return errors.New(adminUsage)
	}
	name := args[0]
	if cmd == "create" {
		return adminCreate(ctx, svc, name, slices.Contains(args[1:], "--admin"), out)
	}
	u, err := svc.Store.Q(ctx).UserByName(name)
	if errors.Is(err, db.ErrNotFound) {
		return fmt.Errorf("no user %q", name)
	}
	if err != nil {
		return err
	}
	switch cmd {
	case "reset-password":
		password := service.RandomPassword()
		if err := svc.ResetPassword(ctx, u.ID, password); err != nil {
			return err
		}
		fmt.Fprintf(out, "new password for %s: %s\n", u.Username, password)
		return nil
	case "promote":
		return adminRole(ctx, svc, u, model.RoleAdmin, out)
	case "demote":
		return adminRole(ctx, svc, u, model.RoleUser, out)
	}
	return errors.New(adminUsage)
}

func adminList(ctx context.Context, svc *service.Service, out io.Writer) error {
	users, err := svc.Store.Q(ctx).Users()
	if err != nil {
		return err
	}
	for _, u := range users {
		state := "active"
		if u.Disabled {
			state = "disabled"
		}
		fmt.Fprintf(out, "%-24s %-6s %s\n", u.Username, u.Role, state)
	}
	return nil
}

func adminCreate(ctx context.Context, svc *service.Service, name string, admin bool, out io.Writer) error {
	role := model.RoleUser
	if admin {
		role = model.RoleAdmin
	}
	password := service.RandomPassword()
	if _, err := svc.CreateUser(ctx, name, password, role); err != nil {
		return err
	}
	fmt.Fprintf(out, "created %s (%s), password: %s\n", name, role, password)
	return nil
}

func adminRole(ctx context.Context, svc *service.Service, u model.User, role string, out io.Writer) error {
	if err := svc.SetRole(ctx, u.ID, role); err != nil {
		if errors.Is(err, service.ErrLastAdmin) {
			return fmt.Errorf("%s is the last active admin", u.Username)
		}
		return err
	}
	fmt.Fprintf(out, "%s is now %s\n", u.Username, role)
	return nil
}
