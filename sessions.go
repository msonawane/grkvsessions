package grkvsessions

import (
	"context"
	"encoding/base32"
	"errors"
	"net/http"
	"strings"

	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"github.com/msonawane/grkv"
	"github.com/msonawane/grkv/kvpb"
)

const (
	sessionKeyPrefix = "session::"
)

type SessionStore struct {
	Codecs  []securecookie.Codec
	Options *sessions.Options
	KV      *grkv.Store
}

// implement gorilla sessions.Store Interface.
var _ sessions.Store = &SessionStore{}

// Keys are defined in pairs to allow key rotation, but the common case is
// to set a single authentication key and optionally an encryption key.
//
// The first key in a pair is used for authentication and the second for
// encryption. The encryption key can be set to nil or omitted in the last
// pair, but the authentication key is required in all pairs.
//
// It is recommended to use an authentication key with 32 or 64 bytes.
// The encryption key, if set, must be either 16, 24, or 32 bytes to select
// AES-128, AES-192, or AES-256 modes.
func NewSessionStore(kv *grkv.Store, keyPairs ...[]byte) *SessionStore {
	ss := &SessionStore{
		Codecs: securecookie.CodecsFromPairs(keyPairs...),
		Options: &sessions.Options{
			Path:   "/",
			MaxAge: 86400 * 30,
		},
		KV: kv,
	}
	return ss
}

// Get returns a session for the given name after adding it to the registry.
// It returns a new session if the sessions doesn't exist. Access IsNew on
// the session to check if it is an existing session or a new one.
// It returns a new session and an error if the session exists but could not be decoded.
func (s *SessionStore) Get(r *http.Request, name string) (*sessions.Session, error) {
	return sessions.GetRegistry(r).Get(s, name)
}

// New returns a session for the given name without adding it to the registry.
//
// The difference between New() and Get() is that calling New() twice will
// decode the session data twice, while Get() registers and reuses the same
// decoded session after the first call.
func (s *SessionStore) New(r *http.Request, name string) (*sessions.Session, error) {
	session := sessions.NewSession(s, name)
	opts := *s.Options
	session.Options = &opts
	session.IsNew = true
	if cook, errCookie := r.Cookie(name); errCookie == nil {
		err := securecookie.DecodeMulti(name, cook.Value, &session.ID, s.Codecs...)
		if err == nil {
			err = s.load(session)
			if err == nil {
				session.IsNew = false
			}
		}
	}
	return session, nil
}

func (s *SessionStore) Save(r *http.Request, w http.ResponseWriter, session *sessions.Session) error {
	// Delete if max-age is <= 0
	if session.Options.MaxAge <= 0 {
		if err := s.erase(session); err != nil {
			return err
		}
		http.SetCookie(w, sessions.NewCookie(session.Name(), "", session.Options))
		return nil
	}

	if session.ID == "" {
		session.ID = strings.TrimRight(
			base32.StdEncoding.EncodeToString(
				securecookie.GenerateRandomKey(32)), "=")
	}
	if err := s.save(session); err != nil {
		return err
	}
	encoded, err := securecookie.EncodeMulti(session.Name(), session.ID,
		s.Codecs...)
	if err != nil {
		return err
	}
	http.SetCookie(w, sessions.NewCookie(session.Name(), encoded, session.Options))
	return nil
}

func (s *SessionStore) load(session *sessions.Session) error {
	resp, err := s.KV.GetWithStringKeys(context.Background(), sessionKeyPrefix+session.ID)
	if err != nil || len(resp.KeysNotFound) != 0 {
		return err
	}

	return securecookie.DecodeMulti(session.Name(), string(resp.Data[0].Value), &session.Values, s.Codecs...)

}

func (s *SessionStore) erase(session *sessions.Session) error {
	dr := &kvpb.DeleteRequest{
		Keys: [][]byte{[]byte(sessionKeyPrefix + session.ID)},
	}
	success, err := s.KV.Delete(context.Background(), dr)
	if success.Success {
		return err
	}
	return errors.New("Error deleting")

}

func (s *SessionStore) save(session *sessions.Session) error {
	encoded, err := securecookie.EncodeMulti(session.Name(), session.Values,
		s.Codecs...)
	if err != nil {
		return err
	}
	data := make([]*kvpb.KeyValue, 0, 1)
	data = append(data, &kvpb.KeyValue{
		Key:       []byte(sessionKeyPrefix + session.ID),
		Value:     []byte(encoded),
		ExpiresAt: uint64(s.Options.MaxAge),
	})
	sr := &kvpb.SetRequest{
		Data: data,
	}
	_, err = s.KV.Set(context.Background(), sr)
	return err

}
