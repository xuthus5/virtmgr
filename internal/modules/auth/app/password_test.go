package app

import "testing"

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if !VerifyPassword(hash, "correct horse battery staple") {
		t.Fatal("VerifyPassword() = false, want true")
	}
	if VerifyPassword(hash, "wrong") {
		t.Fatal("VerifyPassword() accepted wrong password")
	}
}

func TestPasswordRejectsMalformedHash(t *testing.T) {
	for _, hash := range []string{"", "plain", "$argon2id$v=19$m=0,t=3,p=2$abc$def"} {
		if VerifyPassword(hash, "anything") {
			t.Fatalf("VerifyPassword(%q) = true", hash)
		}
	}
}

func TestHashPasswordRejectsEmptyPassword(t *testing.T) {
	if _, err := HashPassword(""); err == nil {
		t.Fatal("HashPassword(\"\") error = nil")
	}
}
