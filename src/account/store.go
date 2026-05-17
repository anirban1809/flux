package account

import (
	"bytes"
	"time"
	"zipcode/src/config"
	"zipcode/src/credentials"

	"github.com/BurntSushi/toml"
)

type AccountStore struct {
	Account *Account
}

type Account struct {
	AccountID     string
	Email         string
	AccessToken   string
	OAuthProvider string // "github" | "google"
	DeviceID      string // server-issued, identifies this install
	SignedInAt    time.Time
}

func (a *AccountStore) Load() error {
	var account Account
	_, err := toml.DecodeFile(config.Cfg.AccountPath, &account)
	if err != nil {
		return err
	}
	a.Account = &account
	return nil
}

func (a *AccountStore) Save() error {
	account := a.Account
	var content bytes.Buffer
	if err := toml.NewEncoder(&content).Encode(account); err != nil {
		return err
	}
	err := credentials.AtomicWrite(config.Cfg.AccountPath, content.Bytes())
	if err != nil {
		return err
	}
	return nil
}

func (a *AccountStore) Clear() error {
	err := credentials.AtomicWrite(config.Cfg.AccountPath, []byte{})
	if err != nil {
		return err
	}
	return nil
}
