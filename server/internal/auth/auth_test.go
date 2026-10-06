package auth

import "testing"

func TestPasswordsAndTokens(t *testing.T) {
	hash, e := Password("secure-password-123")
	if e != nil {
		t.Fatal(e)
	}
	if !Verify(hash, "secure-password-123") || Verify(hash, "wrong") || Verify("malformed", "test") {
		t.Fatal("password verification")
	}
	other, e := Password("secure-password-123")
	if e != nil || other == hash {
		t.Fatal("salt not random")
	}
	token := Token()
	if len(token) != 43 || Hash(token) == token || len(Hash(token)) != 64 || token == Token() {
		t.Fatal("invalid token")
	}
}
