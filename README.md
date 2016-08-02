# Configuration controller for Kubernetes

**This is a simple helper application for Kubernetes that reloads an application whenever a configmap changes**.

Under Kubernetes, configmaps mounted into pods generally require an application to be redeployed in order for the application to pick up changes.

This is, of course, often desirable when the application has a release-oriented lifecycle. However, for long-running infrastructure-oriented apps — some examples include Prometheus, Nginx, PostgreSQL and HAProxy — the lifecycle is not released-oriented, but configuration-oriented. For example, adding a new alert to Prometheus should not require a redeploy.

This controller solves this problem by monitoring the configmap for changes and populating a folder with its contents, then reloading the application.

## Operation

The controller will unpack the configmap to a directory of your choice, and then signal the application. All files referenced in the configmap are copied relative to a single root directory. Files can be relative; intermediate directories will be automatically created.

The controller supports two different modes of operation:

* **Sidecar mode** (preferred) — run the controller in a separate container, with a shared volume where the configmap can be unpacked. To signal the application, an HTTP endpoint must be used.
* **Subprocess mode** — run the controller in the same container, which will execute the application as a subprocess. The application must support the `SIGHUP` signal to reload its configuration.

## Running

### Sidecar mode

In this mode, we run the controller as a sidecar container to the actual application we want to control. This has two requirements:

* The application must be reloadable via HTTP(S).
* The configuration volume is shared between the sidecar controller and the application's container.

#### Example usage (excerpt from pod spec):

```yaml
containers:
- name: prometheus
  image: quay.io/prometheus/prometheus:latest
- name: config-controller
  image: atombender/k8s-config-controller:
  args:
  - --configroot=/mnt/config
  - --configmap=production/prometheus
  - --reload-http=http://localhost:9090/-/reload
  - --reload-http-method=POST
```

#### Required parameters

* `--configroot=DIR` — the root directory relative to which configmap files are unpacked.
* `--configmap=[NS/]NAME` — name of configmap, e.g. `production/postgresql`. The namespace defaults to `default`.
* `--reload-http=URL` — URL to issue reload request with.
* `--reload-http-method=METHOD` — HTTP method to use. Default is `POST`.

Other standard Go and Kubernetes flags may be specified, such as `--v` and `--kubeconfig`. Run with `--help` for usage.

### Subprocess mode

In this mode, we run the controller as the parent process of the application. Requirements:

* The controlled application must support reloading its configuration on either `SIGHUP` or an HTTP request.
* Health checks will be done against the controlled application.
* All config files are written with `0666`.

Unfortunately, in this mode i's necessary to build a container for each application that should run under the configuration controller. Since pods currently don't share a PID namespace, it's not possible for a sidecar container to signal another container.

#### Example usage (excerpt from pod spec):

```yaml
containers:
- name: postgresql
  image: my-postgresql-image
  command: /srv/config-controller
  args:
  - --configroot=/config
  - --configmap=production/postgresql
  - --
  - /usr/lib/postgresql/9.3/bin/postgres
  - -c
  - config_file=/config/postgresql.conf
```

#### Required parameters

* `--configroot=DIR` — the root directory relative to which configmap files are unpacked.
* `--configmap=[NS/]NAME` — name of configmap, e.g. `production/postgresql`. The namespace defaults to `default`.

Additionally, everything following `--` is the command line of the program to start. If you need to refer to a specific configuration file, you can do so.

Other standard Go and Kubernetes flags may be specified, such as `--v` and `--kubeconfig`. Run with `--help` for usage.

Note that the controller will fail fast; if, for example, the controlled application fails, the controller will also fail, on the assumption that Kubernetes will handle recovery.

## Building

Due to [this issue](https://github.com/kubernetes/kubernetes/issues/25572) and bugs in Glide that prevents it from being usable with Kubernetes, this project does not use Glide. Get dependencies with `go get .` after cloning.

Then:

```shell
go build -o config-controller
```

## Limitations

Does not expose a health check API. We should add this so that if the reload request fails, we can reject the health check, which will cause Kubernetes to kill our pod and do a hard restart of it.

## Acknowledgements

I used [Nginx ingress controller](https://github.com/kubernetes/contrib/tree/master/ingress/controllers/nginx) as inspiration, and borrowed small bits of boilerplate code from it, with thanks.
