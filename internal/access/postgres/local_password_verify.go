package postgres

import (
	"fmt"
	"sync"

	"github.com/alexedwards/argon2id"
)

// comparePasswordAndHash is a variable so tests can observe Argon2id work.
var comparePasswordAndHash = argon2id.ComparePasswordAndHash

func verifySecret(value string, verifier []byte) bool {
	ok, err := comparePasswordAndHash(value, string(verifier))
	return err == nil && ok
}

// dummyVerifier is a verifier built once with the production parameters. Login
// verifies against it when the email is unknown so that response time does not
// reveal which emails have local credentials.
var dummyVerifier = sync.OnceValue(func() []byte {
	secret, err := tokenSecret("lv_dummy_")
	if err != nil {
		panic(fmt.Sprintf("generate dummy password: %v", err))
	}
	verifier, err := secretVerifier(secret)
	if err != nil {
		panic(fmt.Sprintf("create dummy password verifier: %v", err))
	}
	return verifier
})
