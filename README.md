# Configuration controller for Kubernetes

**This is a simple helper application for Kubernetes that is designed to reload an application whenever its configmap changes**.

Under Kubernetes, configmaps mounted into pods are immutable; they're snapshots of the version of the configmap that existed when the pod launched. To make the application reflect any changes to a configmap, you normally have to redeploy the application.

This is, of course, often desirable when the application has a release-oriented lifecycle. However, for long-running infrastructure-oriented apps — some examples include Prometheus, Nginx, PostgreSQL and HAProxy — the lifecycle is not always released-oriented, but configuration-oriented. For example, adding a new alert to Prometheus should not require a redeploy.

This controller solves this problem by monitoring the configmap for changes, populating a folder with its contents, and signaling the application to reload.

The controller will fail fast; if, for example, the controlled application fails, the controller will also fail, on the assumption that Kubernetes will handle recovery.

## Requirements and assumptions

* The controlled application must respond to `SIGHUP`.
* All files referenced in the configmap are copied relative to a single root directory. Files can be relative; intermediate directories will be automatically created.
* Health checks will be done against the controlled application.
* All config files are written with `0666`.

## Limitations

Unfortunately, it's necessary to build a container for each application that should run under the configuration controller. Since pods currently don't share a PID namespace, it's not possible for a sidecar container to signal another container.

## Running

Example usage (excerpt from pod spec):

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

Required parameters:

* `--configroot=DIR` — the root directory relative to which configmap files are unpacked.
* `--configmap=[NS/]NAME` — name of configmap, e.g. `production/postgresql`. The namespace defaults to `default`.

Additionally, everything following `--` is the command line of the program to start. If you need to refer to a specific configuration file, you can do so.

Other standard Go and Kubernetes flags may be specified, such as `--v` and `--kubeconfig`. Run with `--help` for usage.

## Building

Note that due to [this issue](https://github.com/kubernetes/kubernetes/issues/25572), dependencies have to be installed with:

```shell
glide install --strip-vendor --strip-vcs
```

Then:

```shell
go build -o config-controller
```

## Acknowledgements

I used [Nginx ingress controller](https://github.com/kubernetes/contrib/tree/master/ingress/controllers/nginx) as inspiration, and borrowed small bits of boilerplate code from it, with thanks.
