package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	h, err := HashPassword("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if h == "hunter2" {
		t.Fatal("hash should not equal plaintext")
	}
	if err := VerifyPassword(h, "hunter2"); err != nil {
		t.Errorf("expected verify ok, got %v", err)
	}
	if err := VerifyPassword(h, "wrong"); err == nil {
		t.Errorf("expected verify to fail on wrong password")
	}
}
