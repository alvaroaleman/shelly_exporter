FROM scratch

COPY shelly-exporter /shelly-exporter

ENTRYPOINT ["/shelly-exporter"]
