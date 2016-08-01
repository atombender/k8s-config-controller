FROM alpine:3.4

MAINTAINER Alexander Staubo <docker@purefiction.net>
LABEL short-description="Configuration controller for Kubernetes"
LABEL full-description="This signals an HTTP endpoint whenever a configmap changes."

ENV \
  CONFIG_CONTROLLER_CONFIGROOT=/config \
  CONFIG_CONTROLLER_CONFIGMAP= \
  CONFIG_CONTROLLER_URL= \
  CONFIG_CONTROLLER_METHOD=POST \
  GO15VENDOREXPERIMENT=1

COPY build/config-controller start-config-controller /bin/

ENTRYPOINT ["/bin/start-config-controller"]
