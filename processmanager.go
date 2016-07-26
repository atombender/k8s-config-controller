package main

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/util/flowcontrol"
)

type processManagerOptions struct {
	command    string
	args       []string
	configRoot string
	errorCh    chan error
}

type processManager struct {
	processManagerOptions
	reloadRateLimiter flowcontrol.RateLimiter
	reloadLock        sync.Mutex
	process           *os.Process
	stopping          bool
	writtenFiles      map[string]struct{}
}

func newProcessManager(options processManagerOptions) (*processManager, error) {
	if options.configRoot == "" {
		return nil, errors.New("Configuration root directory must be set")
	}

	if err := os.MkdirAll(options.configRoot, os.FileMode(0666)); err != nil {
		return nil, errors.Wrapf(err, "Unable to create config root directory %q", options.configRoot)
	}
	return &processManager{
		processManagerOptions: options,
		reloadRateLimiter:     flowcontrol.NewTokenBucketRateLimiter(0.1, 1),
		writtenFiles:          make(map[string]struct{}),
	}, nil
}

func (pm *processManager) Start() error {
	glog.Infof("Starting child process with args: %s", strings.Join(pm.args, " "))

	cmd := exec.Command(pm.command, pm.args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		glog.Errorf("Child process failed to start: %v", err)
		return err
	}

	pm.process = cmd.Process

	go func() {
		err := cmd.Wait()
		if err != nil {
			glog.Errorf("Child process failed: %v", err)
		} else {
			glog.Infof("Child process terminated: %s", cmd.ProcessState.String())
		}
		if !pm.stopping {
			pm.errorCh <- err
		}
		pm.process = nil
	}()

	return nil
}

func (pm *processManager) Stop() {
	if !pm.stopping {
		pm.stopping = true
		if pm.process != nil {
			glog.Info("Sending SIGTERM to child process")
			if err := pm.process.Kill(); err != nil {
				glog.Errorf("Kill failed: %s", err)
			}
		}
	}
}

func (pm *processManager) PopulateDirectoryFromConfigMap(cm api.ConfigMap) error {
	if pm.stopping {
		return nil
	}

	pm.reloadRateLimiter.Accept()

	pm.reloadLock.Lock()
	defer pm.reloadLock.Unlock()

	for fileName, _ := range pm.writtenFiles {
		if err := os.Remove(fileName); err != nil {
			if os.IsNotExist(err) {
				glog.Warningf("Obsolete file unexpectedly does not exist (ignoring): %s", fileName)
			} else {
				return errors.Wrapf(err, "Could not delete obsolete file %s", fileName)
			}
		}
	}

	for name, data := range cm.Data {
		fileName := filepath.Join(pm.configRoot, name)
		glog.Infof("Writing %s", fileName)
		if err := os.MkdirAll(filepath.Dir(fileName), os.FileMode(0666)); err != nil {
			return errors.Wrapf(err, "Could not create parent directory for %s", fileName)
		}
		if err := ioutil.WriteFile(fileName, []byte(data), os.FileMode(0666)); err != nil {
			return errors.Wrapf(err, "Could not write file %s", fileName)
		}
		pm.writtenFiles[fileName] = struct{}{}
	}

	return pm.reload()
}

func (pm *processManager) reload() error {
	if pm.process != nil {
		glog.Info("Sending SIGHUP to application")
		return pm.process.Signal(syscall.SIGHUP)
	}
	return nil
}
