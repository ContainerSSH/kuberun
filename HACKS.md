# Hacks

This file describes the ugly hacks we had to do in this library.

## Kubernetes doesn't handle window resizes synchronously

In the [test code](integration_test.go) we have a sleep of 200 ms after resizing the window. This is done because the Kubernetes client library [handles resize requests asynchronously](https://github.com/kubernetes/client-go/blob/master/tools/remotecommand/v3.go#L69). This causes a race condition where window requests might not arrive before the `tput cols` command is run to test if the window resize was successful.

**Remove:** If the Kubernetes streaming library gets changed to synchronous.
