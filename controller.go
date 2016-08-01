package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/cache"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/controller/framework"
	"k8s.io/kubernetes/pkg/runtime"
	"k8s.io/kubernetes/pkg/util/flowcontrol"
	"k8s.io/kubernetes/pkg/watch"
)

type controllerOptions struct {
	client        *unversioned.Client
	configRootDir string
	configMapName string
	namespace     string
	reloadable    Reloadable
}

type controller struct {
	controllerOptions
	configMapController *framework.Controller
	reloadRateLimiter   flowcontrol.RateLimiter
	reloadLock          sync.Mutex
	writtenFiles        map[string]struct{}
	stopCh              chan struct{}
	stopping            bool
}

func newController(options controllerOptions) (*controller, error) {
	if options.configRootDir == "" {
		return nil, errors.New("Configuration root directory must be set")
	}

	if err := os.MkdirAll(options.configRootDir, os.FileMode(0666)); err != nil {
		return nil, errors.Wrapf(err, "Unable to create config root directory %q", options.configRootDir)
	}

	ctl := &controller{
		controllerOptions: options,
		stopCh:            make(chan struct{}),
		reloadRateLimiter: flowcontrol.NewTokenBucketRateLimiter(0.1, 1),
		writtenFiles:      make(map[string]struct{}),
	}

	configMap, err := ctl.client.ConfigMaps(ctl.namespace).Get(ctl.configMapName)
	if err != nil {
		return nil, errors.Wrapf(err, "Unable to get configmap %q in namespace %q",
			ctl.configMapName, ctl.namespace)
	}

	if err := ctl.updateFromConfigMap(*configMap); err != nil {
		return nil, fmt.Errorf("Unable to populate configuration directory: %s", err)
	}

	mapEventHandler := framework.ResourceEventHandlerFuncs{
		UpdateFunc: func(old, cur interface{}) {
			if !reflect.DeepEqual(old, cur) {
				curMap := cur.(*api.ConfigMap)
				if curMap.Namespace == ctl.namespace &&
					curMap.Name == ctl.configMapName {
					if err := ctl.updateFromConfigMap(*curMap); err != nil {
						glog.Errorf("Unable to populate configuration directory: %s", err)
					}
				}
			}
		},
	}

	_, ctl.configMapController = framework.NewInformer(
		&cache.ListWatch{
			ListFunc: func(opts api.ListOptions) (runtime.Object, error) {
				return ctl.client.ConfigMaps(ctl.namespace).List(opts)
			},
			WatchFunc: func(options api.ListOptions) (watch.Interface, error) {
				return ctl.client.ConfigMaps(ctl.namespace).Watch(options)
			},
		},
		&api.ConfigMap{}, 30*time.Second, mapEventHandler)

	return ctl, nil
}

func (ctl *controller) Stop() error {
	if ctl.stopping {
		return nil
	}

	ctl.stopping = true
	close(ctl.stopCh)

	if stoppable, ok := ctl.reloadable.(Stoppable); ok {
		stoppable.Stop()
	}
	return nil
}

func (ctl *controller) Run() error {
	go ctl.configMapController.Run(ctl.stopCh)
	<-ctl.stopCh
	return nil
}

func (ctl *controller) updateFromConfigMap(cm api.ConfigMap) error {
	if ctl.stopping {
		return nil
	}

	ctl.reloadRateLimiter.Accept()

	ctl.reloadLock.Lock()
	defer ctl.reloadLock.Unlock()

	for fileName, _ := range ctl.writtenFiles {
		if err := os.Remove(fileName); err != nil {
			if os.IsNotExist(err) {
				glog.Warningf("Obsolete file unexpectedly does not exist (ignoring): %s", fileName)
			} else {
				return errors.Wrapf(err, "Could not delete obsolete file %s", fileName)
			}
		}
	}

	for name, data := range cm.Data {
		fileName := filepath.Join(ctl.configRootDir, name)
		glog.Infof("Writing %s", fileName)
		if err := os.MkdirAll(filepath.Dir(fileName), os.FileMode(0666)); err != nil {
			return errors.Wrapf(err, "Could not create parent directory for %s", fileName)
		}
		if err := ioutil.WriteFile(fileName, []byte(data), os.FileMode(0666)); err != nil {
			return errors.Wrapf(err, "Could not write file %s", fileName)
		}
		ctl.writtenFiles[fileName] = struct{}{}
	}

	if err := ctl.reloadable.Reload(); err != nil {
		return errors.Wrapf(err, "Unable to reload")
	}

	return nil
}
