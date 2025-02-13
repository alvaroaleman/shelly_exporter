# Shelly Exporter

A simple Prometheus exporter for Shelly devices. Tested only with Shelly Plus Plug US.

Usage:
```
docker run -v $(pwd)/config.sample.yaml:/config.sample.yaml ghcr.io/alvaroaleman/shelly_exporter --config=/config.sample.yaml
```

or deploy to Kubernetes using the included `./deployment.yaml`
