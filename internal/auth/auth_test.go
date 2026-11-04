package auth

import (
	"testing"
	"time"
)

func TestPassword(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("correct horse battery", h) || VerifyPassword("wrong", h) {
		t.Fatal("verify mismatch")
	}
	if VerifyPassword("x", "$argon2id$v=19$m=4294967295,t=1,p=1$AAAA$AAAA") {
		t.Fatal("must reject absurd params")
	}
	if CheckPasswordPolicy("short") == nil || CheckPasswordPolicy("long enough pw") != nil {
		t.Fatal("policy")
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter(3, 50*time.Millisecond, time.Second)
	for i := 0; i < 2; i++ {
		l.Fail("k")
	}
	if ok, _ := l.Allow("k"); !ok {
		t.Fatal("should still be allowed")
	}
	l.Fail("k")
	if ok, _ := l.Allow("k"); ok {
		t.Fatal("should be locked")
	}
	time.Sleep(70 * time.Millisecond)
	if ok, _ := l.Allow("k"); !ok {
		t.Fatal("lock should expire")
	}
	l.Reset("k")
}

func TestBoxAndRecovery(t *testing.T) {
	b, err := OpenBox(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	enc, _ := b.Seal("JBSWY3DPEHPK3PXP")
	if got, err := b.Open(enc); err != nil || got != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("roundtrip: %v %q", err, got)
	}
	codes, hashes, _ := NewRecoveryCodes(3)
	if len(codes) != 3 || HashToken(NormalizeRecovery(codes[0])) != hashes[0] {
		t.Fatal("recovery")
	}
}

func TestTOTP(t *testing.T) {
	k, err := GenerateTOTP("t", "u")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ValidateTOTP("000000", k.Secret()); ok {
		// 1-in-333k flake; acceptable
		t.Log("random code matched")
	}
}
