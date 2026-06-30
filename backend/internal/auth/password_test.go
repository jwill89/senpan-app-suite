package auth

import "testing"

func TestHashVerifyRoundTrip(t *testing.T) {
	encoded, err := Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	ok, err := Verify("correct horse battery staple", encoded)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !ok {
		t.Fatal("expected the correct password to verify")
	}

	ok, err = Verify("wrong password", encoded)
	if err != nil {
		t.Fatalf("Verify (wrong): %v", err)
	}
	if ok {
		t.Fatal("expected a wrong password to fail verification")
	}
}

func TestHashIsSaltedAndPHCFormatted(t *testing.T) {
	a, err := Hash("same")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Hash("same")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("expected different salts to produce different hashes")
	}
	if len(a) < 8 || a[:9] != "$argon2id" {
		t.Fatalf("expected argon2id PHC prefix, got %q", a)
	}
}

func TestVerifyRejectsMalformedHash(t *testing.T) {
	for _, bad := range []string{"", "plaintext", "$argon2id$bad", "$bcrypt$v=19$..."} {
		if ok, err := Verify("x", bad); ok || err == nil {
			t.Fatalf("expected malformed hash %q to error, got ok=%v err=%v", bad, ok, err)
		}
	}
}
