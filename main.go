package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/golang/glog"
	"github.com/spf13/pflag"
	"k8s.io/kubernetes/pkg/client/unversioned"
	kubectlutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
)

var (
	flags = pflag.NewFlagSet("", pflag.ExitOnError)

	inCluster = flags.Bool("running-in-cluster", true,
		`If this controller is running in a kubernetes cluster, use the pod secrets for
		creating a Kubernetes client.`)

	configMapArg = flags.String("configmap", "",
		`Name of the ConfigMap that contains the Prometheus configuration`)

	configRootDir = flags.String("configroot", "", `Location where configmap will be mounted`)

	reloadHTTPURL = flags.String("reload-http", "", `HTTP endpoint to invoke to reload`)

	reloadHTTPMethod = flags.String("reload-http-method", "POST", `HTTP method to use to reload`)
)

func main() {
	clientConfig := kubectlutil.DefaultClientConfig(flags)

	flags.AddGoFlagSet(flag.CommandLine)
	flags.Parse(os.Args)

	if configMapArg == nil || *configMapArg == "" {
		glog.Fatal("Name of configmap must be specified")
	}

	if configRootDir == nil || *configRootDir == "" {
		glog.Fatal("Configuration root directory must be specified")
	}

	namespace, configMapName, err := parseQualifiedResourceName(*configMapArg)
	if err != nil {
		glog.Fatalf("Invalid configmap name: %s", err)
	}

	var client *unversioned.Client
	if *inCluster {
		client, err = unversioned.NewInCluster()
	} else {
		config, configErr := clientConfig.ClientConfig()
		if configErr != nil {
			glog.Fatalf("Failed to get client configuration: %s", configErr)
		}
		client, err = unversioned.New(config)
	}
	if err != nil {
		glog.Fatalf("Could not create the client: %s", err)
	}

	errorCh := make(chan error)
	go func() {
		err := <-errorCh
		if err != nil {
			glog.Fatalf("Exiting due to process failure: %s", err)
		} else {
			glog.Fatal("Exiting because process exited")
		}
	}()

	var reloadable Reloadable
	if flags.ArgsLenAtDash() == -1 {
		if reloadHTTPURL == nil || *reloadHTTPURL == "" {
			glog.Fatal("Application command line or HTTP endpoint required")
		}
		var method string
		if reloadHTTPMethod != nil && *reloadHTTPMethod != "" {
			method = *reloadHTTPMethod
		}
		reloadable = NewHTTPEndpoint(*reloadHTTPURL, method)
	} else {
		appCommand := flags.Args()[flags.ArgsLenAtDash()]
		appArgs := flags.Args()[flags.ArgsLenAtDash()+1:]
		reloadable, err = NewProcessManager(ProcessManagerOptions{
			command: appCommand,
			args:    appArgs,
			errorCh: errorCh,
		})
		if err != nil {
			glog.Fatal(err)
		}
	}

	ctl, err := newController(controllerOptions{
		client:        client,
		namespace:     namespace,
		configMapName: configMapName,
		configRootDir: *configRootDir,
		reloadable:    reloadable,
	})
	if err != nil {
		glog.Fatalf("%s", err)
	}

	go waitForSigterm(ctl)

	glog.Fatalf("%s", ctl.Run())
}

func waitForSigterm(ctl *controller) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGTERM)
	<-signalChan
	glog.Infof("Received SIGTERM, shutting down")

	exitCode := 0
	if err := ctl.Stop(); err != nil {
		glog.Infof("Error during shutdown: %s", err)
		exitCode = 1
	}

	glog.Infof("Exiting with %v", exitCode)
	os.Exit(exitCode)
}
