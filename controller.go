package main

import (
	"fmt"
	"reflect"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/cache"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/controller/framework"
	"k8s.io/kubernetes/pkg/runtime"
	"k8s.io/kubernetes/pkg/watch"
)

type controllerOptions struct {
	client         *unversioned.Client
	configMapName  string
	namespace      string
	processManager *processManager
}

type controller struct {
	controllerOptions
	configMapController *framework.Controller
	stopCh              chan struct{}
	stopping            bool
}

func newController(options controllerOptions) (*controller, error) {
	ctl := &controller{
		controllerOptions: options,
		stopCh:            make(chan struct{}),
	}

	configMap, err := ctl.client.ConfigMaps(ctl.namespace).Get(ctl.configMapName)
	if err != nil {
		return nil, errors.Wrapf(err, "Unable to get configmap %q in namespace %q",
			ctl.configMapName, ctl.namespace)
	}

	if err := ctl.processManager.PopulateDirectoryFromConfigMap(*configMap); err != nil {
		return nil, fmt.Errorf("Unable to populate configuration directory: %s", err)
	}

	mapEventHandler := framework.ResourceEventHandlerFuncs{
		UpdateFunc: func(old, cur interface{}) {
			if !reflect.DeepEqual(old, cur) {
				curMap := cur.(*api.ConfigMap)
				if curMap.Namespace == ctl.namespace &&
					curMap.Name == ctl.configMapName {
					if err := ctl.processManager.PopulateDirectoryFromConfigMap(*curMap); err != nil {
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

	ctl.processManager.Stop()
	return nil
}

func (ctl *controller) Run() error {
	if err := ctl.processManager.Start(); err != nil {
		return err
	}
	go ctl.configMapController.Run(ctl.stopCh)
	<-ctl.stopCh
	return nil
}
