package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/ndeloof/go-garmin/pkg/garmin"
)

func cmdLogin(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	tokens := tokensFlag(fs)
	verbose := fs.Bool("v", false, "verbose login diagnostics")
	_ = fs.Parse(args)

	email, err := promptLine("Email: ")
	if err != nil {
		return err
	}
	password, err := promptPassword("Password: ")
	if err != nil {
		return err
	}

	var opts []garmin.LoginOption
	if *verbose {
		opts = append(opts, garmin.WithLoginLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	}
	creds, err := garmin.LoginWithMFA(ctx, email, password, func(ctx context.Context, method string) (string, error) {
		return promptLine(fmt.Sprintf("2FA code (sent via %s): ", method))
	}, opts...)
	if err != nil {
		return err
	}

	store := garmin.NewFileTokenStore(*tokens)
	if err := store.Save(ctx, creds); err != nil {
		return fmt.Errorf("saving tokens: %w", err)
	}
	fmt.Printf("Tokens written to %s\n", store.Path())

	// Confirm the credential works and greet the user.
	client := garmin.NewClient(creds, garmin.WithTokenStore(store))
	profile, err := client.UserProfile.SocialProfile(ctx)
	if err != nil {
		return fmt.Errorf("tokens saved but profile check failed: %w", err)
	}
	fmt.Printf("Logged in as %s (%s)\n", nonEmpty(profile.FullName, profile.UserName), profile.DisplayName)
	return nil
}

func cmdWhoami(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("whoami", flag.ExitOnError)
	tokens := tokensFlag(fs)
	_ = fs.Parse(args)

	client, store, err := clientFromTokens(ctx, *tokens)
	if err != nil {
		return err
	}
	profile, err := client.UserProfile.SocialProfile(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Tokens:       %s\n", store.Path())
	fmt.Printf("Display name: %s\n", profile.DisplayName)
	fmt.Printf("Full name:    %s\n", profile.FullName)
	fmt.Printf("User name:    %s\n", profile.UserName)
	if exp := client.Credentials().Expiry(); !exp.IsZero() {
		fmt.Printf("Access token: expires %s\n", exp.Local().Format("2006-01-02 15:04:05"))
	}
	return nil
}

func cmdRefresh(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("refresh", flag.ExitOnError)
	tokens := tokensFlag(fs)
	_ = fs.Parse(args)

	store := garmin.NewFileTokenStore(*tokens)
	creds, err := store.Load(ctx)
	if err != nil {
		return err
	}
	refreshed, err := garmin.Refresh(ctx, creds)
	if err != nil {
		return err
	}
	if err := store.Save(ctx, refreshed); err != nil {
		return fmt.Errorf("token refreshed but saving failed (the old refresh token is now invalid!): %w", err)
	}
	fmt.Printf("Tokens refreshed and written to %s (access token expires %s)\n",
		store.Path(), refreshed.Expiry().Local().Format("2006-01-02 15:04:05"))
	return nil
}

func clientFromTokens(ctx context.Context, path string) (*garmin.Client, *garmin.FileTokenStore, error) {
	store := garmin.NewFileTokenStore(path)
	client, err := garmin.NewClientFromStore(ctx, store)
	if err != nil {
		return nil, store, err
	}
	return client, store, nil
}

func promptLine(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// promptPassword reads a line with terminal echo disabled (via stty; falls
// back to plain input when unavailable, e.g. on Windows or when stdin is not
// a terminal).
func promptPassword(prompt string) (string, error) {
	if runtime.GOOS == "windows" {
		return promptLine(prompt)
	}
	if err := sttyEcho(false); err != nil {
		return promptLine(prompt)
	}
	defer func() {
		_ = sttyEcho(true)
		fmt.Fprintln(os.Stderr)
	}()
	return promptLine(prompt)
}

func sttyEcho(on bool) error {
	arg := "-echo"
	if on {
		arg = "echo"
	}
	cmd := exec.Command("stty", arg)
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func nonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
