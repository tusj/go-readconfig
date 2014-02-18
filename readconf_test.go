package readconf

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"testing"
)

const (
	changeCnt          = 500
	timeout            = 2e9
	programName string = "fonts" // Hopefully existing on all unixes
	confName    string = "fonts.conf"
	seed               = 100
)

var (
	home     string
	XDGHome  string
	confPath string
)

// Clearing of environment
// Needed environment has to be set explicitly as it is not local to functions
func init() {
	home = os.Getenv("HOME")
	XDGHome = os.Getenv("XDG_CONFIG_HOME")
	confPath = path.Join("/etc", programName, confName)
	os.Clearenv()
	// TODO Clean up test files
}

func exists(path string) bool {

	_, err := os.Stat(path)
	if err != nil {
		return os.IsExist(err)
	}
	return true
}

// Test if our system config is existing

func TestSystemConfigIsExisting(t *testing.T) {

	if !exists(confPath) {
		t.Fatal(confPath, "does not exist: cannot do testing")
	}
}

// return a setup for a program whose config is existing / nonexisting
func getConfigSetup(confPath, programName string, shouldExist bool) string {

	for {
		programName = string(randStr(10))
		isExisting := exists(path.Join(confPath, programName))
		if shouldExist && !isExisting {
			continue
		}
		if !shouldExist && isExisting {
			continue
		}

		break
	}
	return programName
}

// Test for no existence of config and)no environment
func TestGetNoSysConfig(t *testing.T) {

	programName := getConfigSetup("/etc", programName, false)

	_, err := Get(programName, confName)
	if err == nil {
		t.Fatal("Received nil when no system config exists:", err)
	}

}

// Test for existence o) sysconfig but no user config
func TestGetSysConfig(t *testing.T) {

	if _, err := Get(programName, confName); err != nil {
		t.Fatal("Got error when system config exists:", err)
	}
}

func setEnv(t *testing.T) {

	if err := os.Setenv("HOME", home); err != nil {
		t.Skip("Could not set $HOME:", err)
	}

	if err := os.Setenv("XDG_CONFIG_HOME", XDGHome); err != nil {
		t.Skip("Could not set XDG_CONFIG_HOME:", err)
	}

}

// Test for user environment but no user config
func TestGetTmpConfig(t *testing.T) {

	if !exists(confPath) {
		t.SkipNow()
	}

	// getting system config because no user environment can be found
	if _, err := Get(programName, confName); err != nil {
		t.Fatal("Could not create user config in tmp:", err)
	}

	// set the home
	setEnv(t)
	if _, err := Get(programName, confName); err != nil {
		t.Fatal("Could not get user config:", err)
	}

}

// Test Listen

func TestListen(t *testing.T) {

	conf, err := Get(programName, confName)
	if err != nil {
		t.Fatal("Got error on creating conf:", err)
	}

	listen, err := conf.Listen()
	if err != nil {
		t.Fatal(err)
	}

	r := 0
	fileName := conf.getPath()

	length := 10
	b := make([]byte, length)
	for i := 0; i < changeCnt; i++ {
		copy(b, randStr(length))
		ioutil.WriteFile(fileName, b, os.ModePerm)

		select {
		case err := <-listen.Error:
			t.Error(err)

		case conf := <-listen.Data:
			fmt.Println("got", string(conf))
			if bytes.Compare(conf, b) != 0 {
				t.Error("Could not read the same as what was written. Got:",
					string(conf), "Sent:", string(b))
			}
			r++
		}
	}

	if r != changeCnt {
		t.Error("Should have detected", changeCnt, " file changes, detected:", r, "changes")
	}

}

func randStr(length int) []byte {
	alphanum := "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	var bytes = make([]byte, length)
	for i := range bytes {
		bytes[i] = alphanum[rand.Intn(len(alphanum))]
	}
	return bytes
}
