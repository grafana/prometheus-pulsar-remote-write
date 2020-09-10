# Changelog

## master / unreleased

* [ENHANCEMENT] Add pprof endpoints by default and add parameter `--web.disable-pprof` to disable them. #23

## 0.1.1 / 2020-08-27

* [CHANGE] Use a single producer instead of one per batch. #18
* [ENHANCEMENT] Cleanup resources properly after SIGTERM. #18

## 0.1.0 / 2020-08-08

* [FEATURE] Initial release of this [Prometheus] remote_write adapter for [Pulsar].


[Prometheus]:https://prometheus.io
[Pulsar]:https://pulsar.apache.org