package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const totpPeriod = 30

func GenerateTOTP(issuer, account string) (*otp.Key, error) {
	return totp.Generate(totp.GenerateOpts{Issuer: issuer, AccountName: account, Period: totpPeriod, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
}

// ValidateTOTP accepts the current step ±1 and returns the matched step so the
// caller can reject replays of an already-used code.
func ValidateTOTP(code, secret string) (step int64, ok bool) {
	code = strings.TrimSpace(code)
	now := time.Now()
	for d := -1; d <= 1; d++ {
		t := now.Add(time.Duration(d*totpPeriod) * time.Second)
		good, err := totp.ValidateCustom(code, secret, t, totp.ValidateOpts{Period: totpPeriod, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
		if err == nil && good {
			return t.Unix() / totpPeriod, true
		}
	}
	return 0, false
}

const recoveryAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

// NewRecoveryCodes returns n codes (shown once) and their stored hashes.
func NewRecoveryCodes(n int) (codes, hashes []string, err error) {
	for i := 0; i < n; i++ {
		b := make([]byte, 10)
		if _, err = rand.Read(b); err != nil {
			return
		}
		for j := range b {
			b[j] = recoveryAlphabet[int(b[j])%len(recoveryAlphabet)]
		}
		c := string(b[:5]) + "-" + string(b[5:])
		codes = append(codes, c)
		hashes = append(hashes, HashToken(c))
	}
	return
}

func NormalizeRecovery(c string) string { return strings.ToLower(strings.TrimSpace(c)) }

// Box encrypts small secrets (TOTP seeds) at rest with a key kept beside the DB.
type Box struct{ aead cipher.AEAD }

func OpenBox(dir string) (*Box, error) {
	path := filepath.Join(dir, "secret.key")
	key, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, err
		}
		if err = os.WriteFile(path, key, 0o600); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, errors.New("secret.key must be 32 bytes")
	}
	blk, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(blk)
	if err != nil {
		return nil, err
	}
	return &Box{g}, nil
}

func (b *Box) Seal(plain string) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(b.aead.Seal(nonce, nonce, []byte(plain), nil)), nil
}

func (b *Box) Open(enc string) (string, error) {
	raw, err := base64.RawStdEncoding.DecodeString(enc)
	if err != nil || len(raw) < b.aead.NonceSize() {
		return "", errors.New("bad ciphertext")
	}
	n := b.aead.NonceSize()
	p, err := b.aead.Open(nil, raw[:n], raw[n:], nil)
	return string(p), err
}
