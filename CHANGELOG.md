# Changelog

## 0.9.3: Testing, removed security options

This release includes tests, bugfixes, and removes the `Disable*` options because they will be part of a separate security overlay.

## 0.9.2: Dependency updates (December 11, 2020)

This release updates the [sshserver](https://github.com/containerssh/sshserver) to version `0.9.10` to match the latest API.

It also exposes helper functions for reading local Kubeconfig files.

## 0.9.1: Dependency updates (December 10, 2020)

This release is updated to match [sshserver 0.9.8](https://github.com/containerssh/sshserver). This changes the `connectionID` parameter from a `[]byte` to a `string` to better reflect its contents of printable characters.

It also updates various minor dependencies and brings tests against all supported Kubernetes versions.

## 0.9.0: Port from ContainerSSH 0.3 (December 4, 2020)

This release ports the old kuberun backend from ContainerSSH 0.3. It introduces using "exec" instead of direct running the executed command in containers.

Fixes [containerssh/containerssh#12](https://github.com/ContainerSSH/ContainerSSH/issues/12)
