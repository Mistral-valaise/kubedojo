package sandbox

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"strings"

	"golang.org/x/crypto/ssh"
)

// NewKeyPair generates an ed25519 ssh key pair in OpenSSH format.
func NewKeyPair() (KeyPair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return KeyPair{}, err
	}
	block, err := ssh.MarshalPrivateKey(priv, "kubedojo")
	if err != nil {
		return KeyPair{}, err
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return KeyPair{}, err
	}
	return KeyPair{
		Private: string(pem.EncodeToMemory(block)),
		Public:  strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))) + " kubedojo",
	}, nil
}
