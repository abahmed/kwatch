# Kwatch E2E installation

This overlay applies the production deployment manifest from `deploy/` with
only the image and pull policy changed for the disposable Kind cluster.

The scenario suite deliberately does not use Helm or `kwatch.sh`. Those paths
have separate validation. Keeping this installation small and source-based
ensures a failure in a runtime scenario is not caused by release packaging or
an interactive installer.
