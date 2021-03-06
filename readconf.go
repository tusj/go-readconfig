// Package readconf implements two things abstracts away the need to set and handle a user configuration.
// It tries to set a configuration path according to the existence of $XDG_CONFIG_HOME and falls back to $HOME/.config.
//
// It supports watching for file changes through inotify.
package readconf

import (
	"code.google.com/p/go.exp/inotify"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"sync"
	"time"
)

// Used to send and receive data and read write errors
type ConfigData struct {
	Data  <-chan []byte
	Error <-chan error
}

//  Holds data about a program's configuration.
type Config struct {
	programPath string // Path to the program's config dir
	programName string // Used as the program's config dir
	confName    string // Filename of the program's configuration
	isTemporary bool
	lock        sync.RWMutex
}

// Listens for changes on the configuration, and returns the read configs.
func (c *Config) Listen() (*ConfigData, error) {

	data := make(chan []byte)
	errs := make(chan error)
	conf := ConfigData{data, errs}

	watcher, err := inotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	confName := c.getPath()

	err = watcher.Watch(confName)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			select {
			case ev := <-watcher.Event:
				switch ev.Mask {
				case inotify.IN_MODIFY:
					fallthrough
				case inotify.IN_MOVE_SELF:
					fallthrough
				case inotify.IN_DELETE_SELF:
				default:
					continue
				}
				// ISSUE Fix having to wait to read file after event has happened to get file content
				<-time.After(5e7)

				if newConf, err := c.Read(); err != nil {
					errs <- err
				} else {
					data <- newConf
				}

			// FEATURE could handle writes as well
			// case newConf := <-conf.Data:
			// 	c.Write(newConf)

			case err := <-watcher.Error:
				errs <- err
			}

		}
	}()

	return &conf, nil
}

// Write contents to the configuration
func (c *Config) Write(newConf []byte) (n int, err error) {

	file, err := os.Create(c.getPath())
	if err != nil {
		return 0, err
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	return file.Write(newConf)
}

// Return the contents of the configuration
func (c *Config) Read() ([]byte, error) {

	c.lock.RLock()
	defer c.lock.RUnlock()

	return ioutil.ReadFile(c.getPath())
}

func (c *Config) Exists() bool {
	_, err := os.Stat(c.getPath())
	return err == nil
}

// Get the full path to the configuration file of the program
func (p *Config) getPath() string {
	return path.Join(p.programPath, p.programName, p.confName)
}

func splitPath(fullPath string) (programPath, programName, confName string, err error) {
	dir := path.Dir(fullPath)
	programPath = path.Dir(dir)
	programName = path.Base(dir)
	confName = path.Base(fullPath)

	switch "" {
	case programPath:
		fallthrough
	case programName:
		fallthrough
	case confName:
		err = errors.New("Could not decompose path.")
	}

	return

}

// Copy a configuration to a path
func (c *Config) copyConf(programPath, programName, confName string) (*Config, error) {

	isTmp := false
	if programPath == "/tmp" {
		isTmp = true
	}

	newConf := Config{programPath, programName, confName, isTmp, sync.RWMutex{}}
	err := newConf.read(c)
	return &newConf, err
}

// Returns a copy of the config which relies in /tmp
func (c *Config) makeTmp() (*Config, error) {
	return c.copyConf("/tmp", c.programName, c.confName)
}

// Creates a Config struct if Config exists
func findConfig(configPath, programName, confName string) (*Config, error) {
	conf := Config{configPath, programName, confName, true, sync.RWMutex{}}
	if conf.Exists() {
		return &conf, nil
	}

	return nil, errors.New(fmt.Sprint("Config does not exist in", conf.getPath()))
}

// Returns the system specific Config
func getSysConfig(programName, confName string) (*Config, error) {
	return findConfig("/etc", programName, confName)
}

// Read in another configuration file
func (c *Config) read(from *Config) error {

	// Create parent directories if necessary with full permissions for user, none for the rest
	if err := os.MkdirAll(path.Join(c.programPath, c.programName), 0700); err != nil {
		return err
	}
	// Copy, truncate destination if it exists
	fromFile, err := os.Open(from.getPath())
	if err != nil {
		return err
	}
	defer fromFile.Close()

	toFile, err := os.Create(c.getPath())
	if err != nil {
		return err
	}
	defer toFile.Close()

	if _, err := io.Copy(toFile, fromFile); err != nil {
		return err
	}
	return nil

}

func copySysConfig(programPath, programName, confName string) (*Config, error) {
	sysConf, err := getSysConfig(programName, confName)
	if err != nil {
		return nil, err
	}

	return sysConf.copyConf(programPath, programName, confName)

}

// Give a program name and a configuration file name, and get returned a path with a valid config.
//
// The function tries to find a configuration in the user directory, and copies one from the system
// directory if none is found. If no user directory can be specified, the program uses the system
// configuration.
// It then copies the system configuration to tmp and returns a configuration which can be modified.
// Otherwise, the system configuration is returned.
// If no system configuration can be retrieved, the program returns an error.
func Get(programName, confName string) (*Config, error) {

	programPath := os.Getenv("XDG_CONFIG_HOME")
	if programPath == "" {
		programPath = path.Join(os.Getenv("HOME"), ".config")
	}

	// Managed to set user path, so try to fetch and or create config here
	if programPath != ".config" {
		if conf, err := findConfig(programPath, programName, confName); err != nil {
			userConf, err := copySysConfig(programPath, programName, confName)
			if err == nil {
				return userConf, nil
			}
		} else {
			return conf, nil
		}
	}

	// Try to fetch the system config
	sysConf, err := getSysConfig(programName, confName)
	if err != nil {
		return nil, err
	}

	tmpConf, err := sysConf.makeTmp()
	if err != nil { // Try to copy to tmp
		return sysConf, nil
	}
	return tmpConf, nil
}
