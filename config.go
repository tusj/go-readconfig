// Package readconf implements two things abstracts away the need to set and handle a user configuratio abstracts away the need to set and handle a user configuration. It tries to set a user defined configuration path according to the existence of $XDG_CONFIG_HOME and falls back to $HOME/.config.
//
// It supports watching for file changes through inotify.
package readconf

import (
	"code.google.com/p/go.exp/inotify"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path"
	"sync"
)

// Used to send and receive data and read write errors
type ConfigData struct {
	Data  chan []byte
	Error chan error
}

//  Holds data about a program's configuration.
type ProgramConfig struct {
	programPath string // Path to the program's config dir
	programName string // Used as the program's config dir
	confName    string // Filename of the program's configuration
	isTemporary bool
	lock        sync.RWMutex
}

// Listens for changes on the configuration, and returns the read configs.
// Reads once on start.
// Update: check if it can handle writes
func (c *ProgramConfig) Listen() (*ConfigData, error) {

	conf := ConfigData{make(chan []byte), make(chan error)}

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
				if ev.Mask != inotify.IN_MODIFY {
					continue
				}

				if newConf, err := c.Read(); err != nil {
					conf.Error <- err
				} else {
					conf.Data <- newConf
				}

			case newConf := <-conf.Data:
				c.Set(newConf)

			case err := <-watcher.Error:
				conf.Error <- err
			}
		}
	}()

	// Force an initial read
	watcher.Event <- new(inotify.Event)

	return &conf, nil
}

// Returns a channel which writes to the configuration
// ISSUE Check with dependency on Get()
func (c *ProgramConfig) Set(newConf []byte) error {

	file, err := os.Create(c.getPath())
	if err != nil {
		return err
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	_, err = file.Write(newConf)
	return err
}

// Return a read of the configuration
func (c *ProgramConfig) Read() ([]byte, error) {

	// Make a lock per function

	c.lock.RLock()
	defer c.lock.RUnlock()

	return ioutil.ReadFile(c.getPath())
}

func (c *ProgramConfig) Exists() bool {
	_, err := os.Stat(c.getPath())
	return err != nil
}

// Get the full path to the configuration file of the program
func (p *ProgramConfig) getPath() string {
	return path.Join(p.programPath, p.programName, p.confName)
}

// Returns the ProgramConfig path if it exists
// func (c *ProgramConfig) Get() (string, error) {
// 	if _, err := os.Stat(c.getPath()); err != nil {
// 		return "", err
// 	}
// 	return c.getPath(), nil
// }

// Copy a configuration to a path
func (c *ProgramConfig) copyConf(to string) (*ProgramConfig, error) {
	dir := path.Dir(to)
	programPath := path.Dir(dir)
	confDir := path.Base(dir)
	confName := path.Base(to)

	switch "" {
	case programPath:
		fallthrough
	case confDir:
		fallthrough
	case confName:
		return nil, errors.New("Could not decompose path.")
	}

	isTmp := false
	if programPath == "/tmp" {
		isTmp = true
	}

	newConf := ProgramConfig{programPath, confDir, confName, isTmp, sync.RWMutex{}}

	// Create parent directories if necessary
	if err := os.MkdirAll(dir, os.ModeDir); err != nil {
		return nil, err
	}
	// Copy, truncate destination if it exists
	if fromFile, err := os.Open(c.getPath()); err != nil {
		return nil, err
	} else if toFile, err := os.Create(newConf.getPath()); err != nil {
		return nil, err
	} else if _, err := io.Copy(toFile, fromFile); err != nil {
		return nil, err
	}
	return &newConf, nil
}

// Returns a copy of the config which relies in /tmp
func (c *ProgramConfig) makeTmp() (*ProgramConfig, error) {
	return c.copyConf(path.Join("/tmp", c.programName, c.confName))
}

// Creates a ProgramConfig struct if ProgramConfig exists
func findConfig(configPath, programName, confName string) (*ProgramConfig, error) {
	conf := ProgramConfig{configPath, programName, confName, true, sync.RWMutex{}}
	if conf.Exists() {
		return &conf, nil
	}
	return nil, errors.New("System specific ProgramConfig does not exist")
}

// Returns the system specific ProgramConfig
func getSysConfig(programName, confName string) (*ProgramConfig, error) {
	return findConfig("/etc", programName, confName)
}

// Give a program name and a configuration file name, and get returned a path with a valid config.
//
// The function tries to find a configuration in the user directory, and copies one from the system
// directory if none is found. If no user directory can be specified, the program uses the system
// configuration.
// It then copies the system configuration to tmp and returns a configuration which can be modified.
// Otherwise, the system configuration is returned.
// If no system configuration can be retrieved, the program returns an error.
func Get(programName, confName string) (*ProgramConfig, error) {

	// Try to fetch and or create config in $XDG_CONFIG_HOME
	if t := os.Getenv("XDG_CONFIG_HOME"); t != "" {
		if conf, err := findConfig(t, programName, confName); err != nil {
			return nil, err
		} else {
			return conf, nil
		}
	}

	// Try to fetch and or create config in $HOME/.config
	if t := os.Getenv("HOME"); t != "" {
		if conf, err := findConfig(t, programName, confName); err != nil {
			return nil, err
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
