package grkvsessions

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/msonawane/grkv"
	"go.uber.org/zap"
)

var testSessionStore *SessionStore

func TestMain(m *testing.M) {
	logger, _ := zap.NewDevelopment()
	cfg := &grkv.Options{
		Path:                "./tmp/db",
		NoSync:              false,
		ValueLogGC:          true,
		GCInterval:          10 * time.Minute,
		MandatoryGCInterval: 1 * time.Hour,
		GCThreshold:         10000000,
		GRPCIP:              "127.0.0.1",
		GRPCPort:            8001,
		MLBindAddr:          "127.0.0.1",
		MLBindPort:          8002,
	}
	kv, err := grkv.New(cfg, logger)
	if err != nil {
		logger.Fatal("error creating grkb")
	}

	testSessionStore = NewSessionStore(kv, []byte("some key"))

	exitVal := m.Run()
	_ = testSessionStore.KV.Close()

	os.Exit(exitVal)

}
func TestSessionStore(t *testing.T) {
	originalPath := "/"
	req, err := http.NewRequest("GET", "http://www.example.com", nil)
	ok(t, err)
	session, err := testSessionStore.New(req, "hello")
	ok(t, err)
	testSessionStore.Options.Path = "/foo"
	equals(t, originalPath, session.Options.Path)
}

func TestGH2MaxLength(t *testing.T) {
	req, err := http.NewRequest("GET", "http://www.example.com", nil)
	ok(t, err)
	w := httptest.NewRecorder()
	session, err := testSessionStore.New(req, "hello")
	ok(t, err)
	session.Values["big"] = make([]byte, base64.StdEncoding.DecodedLen(4096*2))
	err = session.Save(req, w)
	shouldError(t, err)
}

func TestStoreDelete(t *testing.T) {
	req, err := http.NewRequest("GET", "http://www.example.com", nil)
	ok(t, err)
	w := httptest.NewRecorder()
	session, err := testSessionStore.New(req, "hello")
	ok(t, err)
	err = session.Save(req, w)
	ok(t, err)
	session.Options.MaxAge = -1
	err = session.Save(req, w)
	ok(t, err)

	session.Options.MaxAge = 0
	err = session.Save(req, w)
	ok(t, err)
}

func TestStoreDelete2(t *testing.T) {
	req, err := http.NewRequest("GET", "http://www.example.com", nil)
	ok(t, err)
	w := httptest.NewRecorder()
	session, err := testSessionStore.New(req, "hello")
	ok(t, err)
	err = session.Save(req, w)
	ok(t, err)
	session.Options.MaxAge = 0
	err = session.Save(req, w)
	ok(t, err)
}

// assert fails the test if the condition is false.
func assert(tb testing.TB, condition bool, msg string, v ...interface{}) {
	if !condition {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d: "+msg+"\033[39m\n\n", append([]interface{}{filepath.Base(file), line}, v...)...)
		tb.FailNow()
	}
}

// ok fails the test if an error is not nil.
func ok(tb testing.TB, err error) {
	if err != nil {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d: unexpected error:%s\033[39m\n\n", filepath.Base(file), line, err.Error())
		tb.FailNow()
	}
}

// shouldError fails the test if an error is  nil.
func shouldError(tb testing.TB, err error) {
	if err == nil {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d: should return an error: \033[39m\n\n", filepath.Base(file), line)
		tb.FailNow()
	}
}

// equals fails the test if exp is not equal to act.
func equals(tb testing.TB, exp, act interface{}) {
	if !reflect.DeepEqual(exp, act) {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d:\n:\n\texp: %#v\n\n\tgot: %#v\033[39m\n\n", filepath.Base(file), line, exp, act)
		tb.FailNow()
	}
}
