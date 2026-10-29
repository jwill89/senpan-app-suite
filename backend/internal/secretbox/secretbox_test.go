package secretbox_test

import (
	"errors"
	"strings"
	"testing"

	"app-suite/internal/secretbox"
)

const purpose = "senpan discord bot token v1"

func box(t *testing.T, secret string) *secretbox.Box {
	t.Helper()
	b, err := secretbox.New(secret, purpose)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRoundTrip(t *testing.T) {
	b := box(t, "the-session-secret")
	sealed, err := b.Seal("MTIz.bot.token")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sealed, "v1:") || strings.Contains(sealed, "MTIz.bot.token") {
		t.Fatalf("sealed = %q", sealed)
	}
	got, err := b.Open(sealed)
	if err != nil || got != "MTIz.bot.token" {
		t.Fatalf("Open = %q, %v", got, err)
	}
	// Every seal uses a fresh nonce.
	again, _ := b.Seal("MTIz.bot.token")
	if again == sealed {
		t.Fatal("two seals of one value are identical")
	}
}

func TestAnotherSecretCannotOpen(t *testing.T) {
	sealed, _ := box(t, "production-secret").Seal("token")
	if _, err := box(t, "dev-machine-secret").Open(sealed); !errors.Is(err, secretbox.ErrUnreadable) {
		t.Fatalf("Open under another secret = %v, want ErrUnreadable", err)
	}
}

func TestAnotherPurposeCannotOpen(t *testing.T) {
	sealed, _ := box(t, "secret").Seal("token")
	other, err := secretbox.New("secret", "something else v1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Open(sealed); !errors.Is(err, secretbox.ErrUnreadable) {
		t.Fatalf("Open under another purpose = %v, want ErrUnreadable", err)
	}
}

func TestTamperedOrForeignValuesAreUnreadable(t *testing.T) {
	b := box(t, "secret")
	sealed, _ := b.Seal("token")
	tampered := sealed[:len(sealed)-2] + "AA"
	for _, v := range []string{tampered, "v1:", "v1:not-base64!", "plain-token", "", "v2:" + strings.TrimPrefix(sealed, "v1:")} {
		if _, err := b.Open(v); !errors.Is(err, secretbox.ErrUnreadable) {
			t.Errorf("Open(%q) = %v, want ErrUnreadable", v, err)
		}
	}
}

func TestEmptySecretIsRefused(t *testing.T) {
	if _, err := secretbox.New("  ", purpose); err == nil {
		t.Fatal("an empty secret must be refused")
	}
}
