//go:build integration

package integration

import (
	"testing"
)

func TestSocialProfile(t *testing.T) {
	c := testClient(t)
	ctx := testCtx(t)
	p, err := c.UserProfile.SocialProfile(ctx)
	if err != nil {
		t.Fatalf("SocialProfile: %v", err)
	}
	if p.DisplayName == "" {
		t.Fatal("empty displayName")
	}
	t.Logf("logged in as %s (displayName %s)", p.FullName, p.DisplayName)

	dn, err := c.DisplayName(ctx)
	if err != nil || dn != p.DisplayName {
		t.Fatalf("DisplayName cache: %q vs %q (%v)", dn, p.DisplayName, err)
	}
}

func TestUserSettings(t *testing.T) {
	c := testClient(t)
	us, err := c.UserProfile.Settings(testCtx(t))
	if err != nil {
		t.Fatalf("Settings: %v", err)
	}
	if us.UserData.MeasurementSystem == "" {
		t.Error("empty measurementSystem")
	}
	t.Logf("measurement system: %s", us.UserData.MeasurementSystem)
}
