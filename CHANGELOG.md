# Changelog

## master

* [FEATURE] Add consume mode, which consumes metrics on the pulsar bus and send them as remote_write requests. #34

## 0.1.2 / 2020-09-11

* [ENHANCEMENT] Add pprof endpoints by default and add parameter `--web.disable-pprof` to disable them. #23
* [ENHANCEMENT] Add configuration parameter `--web.max-connection-age` to limit lifetime of HTTP connections. #22

## 0.1.1 / 2020-08-27

* [CHANGE] Use a single producer instead of one per batch. #18
* [ENHANCEMENT] Cleanup resources properly after SIGTERM. #18

## 0.1.0 / 2020-08-08

* [FEATURE] Initial release of this [Prometheus] remote_write adapter for [Pulsar].


[Prometheus]:https://prometheus.io
[Pulsar]:https://pulsar.apache.org
